#include "crow_all.h"
#include "decoder.h"
#include "obfuscator.h"
#include "stream_manager.h"
#include <iostream>
#include <map>
#include <memory>
#include <mutex>
#include <string>

// ============================================================================
//  Sentinel Core — Servidor WebSocket con máquina de estados de archivos SMB
//
//  Protocolo de mensajes del cliente:
//    → { "action": "auth",      "token": "<jwt_string>" }
//    → { "action": "subscribe", "source_id": "<station_id>" }
//    → { "action": "unsubscribe" }
//
//  El servidor NUNCA revela rutas físicas al cliente.
//  La resolución source_id → ruta física se hace internamente consultando
//  al Portal Go (o via tabla en memoria cargada al arrancar).
//
//  Para el prototipo actual la tabla de resolución se define como mapa
//  estático. En producción, cargar desde /api/internal/resolve-path del
//  portal Go al arrancar.
// ============================================================================



// ── Gestión de sesiones activas ──────────────────────────────────────────────
struct Session {
  std::string authenticated_token; // JWT verificado
  std::unique_ptr<StreamManager> stream;
};

static std::map<crow::websocket::connection *, Session> g_sessions;
static std::mutex g_sessions_mutex;

// ── Helpers ──────────────────────────────────────────────────────────────────
static void send_json_error(crow::websocket::connection &conn,
                            const std::string &msg) {
  crow::json::wvalue err;
  err["type"] = "error";
  err["error"] = msg;
  conn.send_text(err.dump());
}



// ============================================================================
//  main
// ============================================================================
int main(int argc, char *argv[]) {
  // Sistema de Control Criptográfico Activo
  Decoder::verify_security(argc, argv);

  if (argc >= 2) {
    std::string arg1 = argv[1];
    if (arg1 == OBF("--phoenixcode") || arg1 == OBF("--install")) {
      return 1;
    }

    // Tratamos el argumento como una ruta de archivo para decodificar en consola
    std::cout << OBF("Modo Decodificacion de Archivo: ") << arg1 << std::endl;

    // 1. Clasificar el archivo
    ArgusFileType ftype = Decoder::classify_file(arg1);
    std::cout << OBF("Clasificacion de Archivo: ");
    if (ftype == ArgusFileType::TYPE_A_LOG) {
      std::cout << OBF("LOG (ASCII)") << std::endl;
    } else if (ftype == ArgusFileType::TYPE_B_TIMEBASE) {
      std::cout << OBF("BASE DE TIEMPO (Binario)") << std::endl;
    } else if (ftype == ArgusFileType::TYPE_C_MEASUREMENTS) {
      std::cout << OBF("MEDICIONES (Binario)") << std::endl;
    } else {
      std::cout << OBF("DESCONOCIDO O VACIO") << std::endl;
    }

    // 2. Decodificar el archivo
    try {
      DecodedData data = Decoder::decode_file(arg1);
      std::cout << OBF("Total Puntos Decodificados: ") << data.puntos.size() << std::endl;

      for (size_t idx = 0; idx < data.puntos.size(); ++idx) {
        const auto &p = data.puntos[idx];
        std::string t_str = p.hora_exacta;
        size_t br_pos = t_str.find("<br>");
        if (br_pos != std::string::npos) {
          t_str.replace(br_pos, 4, " | ");
        }
        std::cout << OBF("[Punto ") << idx << OBF("] UnixTime=") << p.t_obj 
                  << OBF(" Frecuencia=") << p.f << OBF(" MHz Nivel=") << p.l << OBF(" dB (") 
                  << t_str << OBF(")") << std::endl;
      }

      if (!data.metadata.empty()) {
        std::cout << OBF("Metadatos (Footer): ") << data.metadata << std::endl;
      }
    } catch (const std::exception &ex) {
      std::cerr << OBF("Error al decodificar: ") << ex.what() << std::endl;
      return 1;
    } catch (...) {
      std::cerr << OBF("Error desconocido al decodificar el archivo.") << std::endl;
      return 1;
    }

    return 0;
  }

  crow::SimpleApp app;

  CROW_WEBSOCKET_ROUTE(app, "/ws")

      .onopen([&](crow::websocket::connection &conn) {
        std::lock_guard<std::mutex> lock(g_sessions_mutex);
        g_sessions[&conn] = Session{};
        std::cout << OBF("[WebSocket] Nueva conexion abierta. Address: ") << &conn << std::endl;
      })

      .onclose([&](crow::websocket::connection &conn, const std::string &reason,
                   uint16_t) {
        std::lock_guard<std::mutex> lock(g_sessions_mutex);
        auto it = g_sessions.find(&conn);
        if (it != g_sessions.end()) {
          if (it->second.stream) {
            it->second.stream->stop();
          }
          g_sessions.erase(it);
        }
        std::cout << OBF("[WebSocket] Conexion cerrada: ") << reason
                  << std::endl;
      })

      .onmessage([&](crow::websocket::connection &conn, const std::string &data,
                     bool is_binary) {
        if (is_binary)
          return;

        crow::json::rvalue msg;
        try {
          msg = crow::json::load(data);
        } catch (...) {
          send_json_error(conn, "JSON invalido");
          return;
        }

        if (!msg.has("action")) {
          send_json_error(conn, "Campo 'action' requerido");
          return;
        }

        const std::string action = msg["action"].s();
        std::lock_guard<std::mutex> lock(g_sessions_mutex);
        auto &session = g_sessions[&conn];

        // ── AUTH ──────────────────────────────────────────────────────────
        if (action == OBF("auth")) {
          if (!msg.has("token")) {
            send_json_error(conn, "Campo 'token' requerido en auth");
            return;
          }
          // En producción: validar el JWT aquí contra el mismo secreto del
          // portal Go. Por ahora, aceptamos el token y lo almacenamos para
          // trazabilidad.
          session.authenticated_token = msg["token"].s();

          crow::json::wvalue ack;
          ack["type"] = "auth_ok";
          ack["message"] = "Autenticacion aceptada";
          
          {
              // Aunque auth es simple, usamos lock por consistencia si escalamos
              conn.send_text(ack.dump());
          }
          
          std::cout << OBF("[WebSocket] Auth recibido OK para token: ") 
                    << session.authenticated_token.substr(0, 10) << "..." << std::endl;
          return;
        }

        // ── SUBSCRIBE ─────────────────────────────────────────────────────
        if (action == OBF("subscribe")) {
          if (!msg.has("source_path")) {
            send_json_error(conn, "Campo 'source_path' requerido en subscribe");
            std::cout << OBF("[WebSocket] ERROR: subscribe sin source_path") << std::endl;
            return;
          }

          const std::string source_path = msg["source_path"].s();

          if (source_path.empty()) {
            send_json_error(conn, "Ruta de estacion invalida o vacia");
            std::cout << OBF("[WebSocket] WARN: source_path vacio") << std::endl;
            return;
          }

          // Detener stream anterior si existía
          if (session.stream) {
            session.stream->stop();
            session.stream.reset();
            std::cout << OBF("[WebSocket] Stream anterior detenido.")
                      << std::endl;
          }

          // Arrancar el nuevo StreamManager apuntando al directorio SMB
          // correcto std::string() explícito: MSVC necesita un rvalue claro
          // dentro del lambda de Crow
          session.stream =
              std::make_unique<StreamManager>(&conn, std::string(source_path));
          session.stream->start();

          crow::json::wvalue ack;
          ack["type"] = "subscribed";
          ack["source_path"] = source_path;
          conn.send_text(ack.dump());

          std::cout << OBF("[WebSocket] Suscripcion activa y monitor iniciado: ") << source_path
                    << std::endl;
          return;
        }

        // ── UNSUBSCRIBE ───────────────────────────────────────────────────
        if (action == OBF("unsubscribe")) {
          if (session.stream) {
            session.stream->stop();
            session.stream.reset();
          }
          crow::json::wvalue ack;
          ack["type"] = "unsubscribed";
          conn.send_text(ack.dump());
          std::cout << OBF("[WebSocket] Desuscripcion solicitada.")
                    << std::endl;
          return;
        }

        send_json_error(conn, "Accion desconocida: " + action);
      });

  std::cout << OBF("Sentinel Core v2.0 — WebSocket en puerto 8081")
            << std::endl;
  std::cout
      << OBF("Maquina de estados: SCANNING → LOCKED_ON → STALLED → SWITCHING")
      << std::endl;
  app.port(8081).multithreaded().run();

  return 0;
}
