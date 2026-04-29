#ifndef SENTINEL_SERVER_H
#define SENTINEL_SERVER_H

#include <QObject>
#include <QWebSocketServer>
#include <QWebSocket>
#include <unordered_map>
#include <memory>
#include <thread>
#include "RadarMonitor.h"
#include "httplib.h"

class SentinelServer : public QObject {
    Q_OBJECT
public:
    explicit SentinelServer(quint16 wsPort, int httpPort, QObject *parent = nullptr);
    ~SentinelServer();

private slots:
    void onNewConnection();
    void processTextMessage(const QString& message);
    void socketDisconnected();

private:
    void startHttpServer(int port);
    void setupMonitor(const std::string& source_id);

    QWebSocketServer *m_pWebSocketServer;
    QList<QWebSocket *> m_clients;
    
    std::unordered_map<std::string, std::shared_ptr<RadarMonitor>> m_monitors;
    std::unordered_map<QWebSocket*, std::string> m_client_subscriptions;

    httplib::Server http_svr;
    std::thread http_thread;
    
    std::string resolvePath(const std::string& source);
};

#endif
