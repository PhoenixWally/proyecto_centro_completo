#include "SentinelServer.h"
#include "ConfiguradorFuentes.h"
#include <QCoreApplication>
#include <iostream>
#include <nlohmann/json.hpp>

using json = nlohmann::json;

SentinelServer::SentinelServer(quint16 wsPort, int httpPort, QObject *parent)
    : QObject(parent), m_pWebSocketServer(new QWebSocketServer(
                           QStringLiteral("Sentinel C++ Server"),
                           QWebSocketServer::NonSecureMode, this)) {

  if (m_pWebSocketServer->listen(QHostAddress::Any, wsPort)) {
    std::cout << "[WS] Servidor Multiplex WebSocket escuchando en el puerto "
              << wsPort << std::endl;
    connect(m_pWebSocketServer, &QWebSocketServer::newConnection, this,
            &SentinelServer::onNewConnection);
  }

  startHttpServer(httpPort);
}

SentinelServer::~SentinelServer() {
  m_pWebSocketServer->close();
  http_svr.stop();
  if (http_thread.joinable())
    http_thread.join();
}

std::string SentinelServer::resolvePath(const std::string &source) {
  auto fuentes = ConfiguradorFuentes::cargarFuentes();
  for (const auto &f : fuentes) {
    if (f.id == source) {
      // Autenticamos contra Windows de manera invisible con usuario y
      // contraseña
      ConfiguradorFuentes::conectarUNC(f.path, f.user, f.password);
      return f.path;
    }
  }
  return "";
}

#include <QDir>

void SentinelServer::startHttpServer(int port) {
  // 1. Buscar primero en la carpeta de despliegue (al lado del exe)
  std::string public_dir =
      QCoreApplication::applicationDirPath().toStdString() + "/public";

  // 2. Si no existe (porque le dimos al "Play" desde Qt Creator y estamos en la
  // subcarpeta build)
  if (!QDir(QString::fromStdString(public_dir)).exists()) {
    public_dir =
        "D:/jpit/Remotas/argus/traduccion_c++/proyecto_web/public"; // Fallback
                                                                    // Modo
                                                                    // Desarrollador
  }

  if (!http_svr.set_mount_point("/", public_dir)) {
    std::cout << "[HTTP] Error CRITICO: No se encontro el directorio estatico "
                 "Front-End en "
              << public_dir << std::endl;
    std::cout << "[HTTP] Asegurese de pegar la carpeta 'public' al lado de "
                 "este programa."
              << std::endl;
  }

  // ================== API REST: FUENTES ==================
  http_svr.Get("/api/fuentes",
               [](const httplib::Request &req, httplib::Response &res) {
                 auto fuentes = ConfiguradorFuentes::cargarFuentes();
                 json j = json::array();
                 for (const auto &f : fuentes) {
                   j.push_back({{"id", f.id},
                                {"path", f.path},
                                {"user", f.user},
                                {"password", f.password}});
                 }
                 res.set_content(j.dump(), "application/json");
               });

  http_svr.Post("/api/fuentes",
                [](const httplib::Request &req, httplib::Response &res) {
                  try {
                    json j = json::parse(req.body);
                    std::vector<FuenteRadar> fuentes;
                    for (const auto &item : j) {
                      FuenteRadar f;
                      f.id = item.value("id", "");
                      f.path = item.value("path", "");
                      f.user = item.value("user", "");
                      f.password = item.value("password", "");
                      fuentes.push_back(f);
                    }
                    ConfiguradorFuentes::guardarFuentes(fuentes);
                    res.set_content("{\"status\":\"ok\"}", "application/json");
                  } catch (...) {
                    res.status = 400;
                  }
                });

  // ================== API REST: CAPTURAS ==================
  http_svr.Post("/api/captura", [](const httplib::Request &req,
                                   httplib::Response &res) {
    try {
      json j = json::parse(req.body);
      std::string src = j.value("src", "UNK");
      std::string b64 = j.value("image", "");
      if (b64.find("base64,") != std::string::npos)
        b64 = b64.substr(b64.find("base64,") + 7);

      // Decodificar base64 puro usando el conversor estático de Qt
      QByteArray binData =
          QByteArray::fromBase64(QByteArray::fromStdString(b64));

      std::filesystem::path dir = std::filesystem::path("capturas") / src;
      std::filesystem::create_directories(dir);

      time_t rawtime;
      struct tm timeinfo;
      char buffer[80];
      time(&rawtime);
      localtime_s(&timeinfo, &rawtime);
      strftime(buffer, sizeof(buffer), "%Y%m%d_%H%M%S", &timeinfo);
      std::string filename = (dir / (std::string(buffer) + ".jpg")).string();

      std::ofstream file(filename, std::ios::binary);
      if (file.is_open())
        file.write(binData.data(), binData.size());

      // Purga Rotativa (solo últimos 100 archivos)
      std::vector<std::filesystem::path> files;
      for (const auto &entry : std::filesystem::directory_iterator(dir)) {
        if (entry.path().extension() == ".jpg")
          files.push_back(entry.path());
      }
      if (files.size() > 100) {
        std::sort(files.begin(), files.end(), [](const auto &a, const auto &b) {
          return std::filesystem::last_write_time(a) <
                 std::filesystem::last_write_time(b);
        });
        int to_delete = files.size() - 100;
        for (int i = 0; i < to_delete; i++)
          std::filesystem::remove(files[i]);
      }

      std::cout << "[Storage] Captura guardada en HD: " << filename
                << std::endl;
      res.set_content("{\"status\":\"ok\"}", "application/json");
    } catch (...) {
      res.status = 500;
    }
  });

  /* ==========================================================
     VERSIÓN ORIGINAL (1 SOLO PUERTO ORIGINAL)
  ========================================================== */
  std::cout << "[HTTP] Servidor Web Estático despachando en http://localhost:"
            << port << std::endl;
  http_thread =
      std::thread([this, port]() { http_svr.listen("0.0.0.0", port); });
}

void SentinelServer::onNewConnection() {
  QWebSocket *pSocket = m_pWebSocketServer->nextPendingConnection();
  std::cout << "[WS] Nuevo visor WebGL conectado al backend." << std::endl;

  connect(pSocket, &QWebSocket::textMessageReceived, this,
          &SentinelServer::processTextMessage);
  connect(pSocket, &QWebSocket::disconnected, this,
          &SentinelServer::socketDisconnected);

  m_clients << pSocket;
}

void SentinelServer::processTextMessage(const QString &message) {
  QWebSocket *pClient = qobject_cast<QWebSocket *>(sender());
  if (!pClient)
    return;

  try {
    json msg = json::parse(message.toStdString());
    if (msg.contains("action")) {
      std::string action = msg["action"];
      if (action == "subscribe" && msg.contains("source")) {
        std::string src = msg["source"];
        m_client_subscriptions[pClient] = src;
        std::cout << "[WS] Cliente pide visualizar en directo la antena: "
                  << src << std::endl;
        setupMonitor(src);
      }
      if (action == "update_filters" && msg.contains("source")) {
        std::string src = msg["source"];
        if (m_monitors.find(src) != m_monitors.end()) {
          std::optional<double> fmin = std::nullopt;
          std::optional<double> fmax = std::nullopt;
          if (!msg["fmin"].is_null())
            fmin = msg["fmin"].get<double>();
          if (!msg["fmax"].is_null())
            fmax = msg["fmax"].get<double>();
          m_monitors[src]->setFrequencyFilter(fmin, fmax);
        }
      }
      if (action == "clear_cache" && msg.contains("source")) {
        std::string src = msg["source"];
        if (m_monitors.find(src) != m_monitors.end()) {
          m_monitors[src]->clearCache();
          std::cout
              << "[Motor] Purgando cache 3D y 2D bajo demanda para origen: "
              << src << std::endl;
        }
      }
    }
  } catch (...) {
  }
}

void SentinelServer::setupMonitor(const std::string &source_id) {
  if (m_monitors.find(source_id) == m_monitors.end()) {
    std::string path = resolvePath(source_id);
    if (path.empty()) {
      std::cout << "[Motor] Ignorando arranque. Origen desconocido: "
                << source_id << std::endl;
      return;
    }

    std::cout << "[Motor] Encendiendo Lector Optimizador Thread para: "
              << source_id << std::endl;
    auto monitor = std::make_shared<RadarMonitor>(source_id, path);

    // Multi-cast Func. Capturamos this para pasarlo al hilo de Qt
    monitor->onDataBroadcast = [this,
                                source_id](const std::string &json_payload) {
      QMetaObject::invokeMethod(this, [this, source_id, json_payload]() {
        QString rq = QString::fromStdString(json_payload);
        for (QWebSocket *client : std::as_const(m_clients)) {
          if (m_client_subscriptions[client] == source_id) {
            client->sendTextMessage(rq);
          }
        }
      });
    };

    // Multi-cast Func para Avisos/Errores en Consola Frontend
    monitor->onLogBroadcast = [this, source_id](const std::string &msg) {
      json alert;
      alert["type"] = "log_msg";
      alert["msg"] = msg;
      QMetaObject::invokeMethod(this, [this, source_id, alert]() {
        QString rq = QString::fromStdString(alert.dump());
        for (QWebSocket *client : std::as_const(m_clients)) {
          if (m_client_subscriptions[client] == source_id) {
            client->sendTextMessage(rq);
          }
        }
      });
    };

    monitor->start();
    m_monitors[source_id] = monitor;
  }
}

void SentinelServer::socketDisconnected() {
  QWebSocket *pClient = qobject_cast<QWebSocket *>(sender());
  if (pClient) {
    std::cout << "[WS] Visor WebGL abandonó la sala." << std::endl;
    m_client_subscriptions.erase(pClient);
    m_clients.removeAll(pClient);
    pClient->deleteLater();
  }
}
