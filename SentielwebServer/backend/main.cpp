#include <QCoreApplication>
#include "SentinelServer.h"
#include <iostream>

int main(int argc, char *argv[]) {
    // Aplicación silenciosa de consola sin QWidgets
    QCoreApplication a(argc, argv);
    
    std::cout << "========================================" << std::endl;
    std::cout << "  Sentinel V3 Server (C++ Qt WebSockets)" << std::endl;
    std::cout << "========================================" << std::endl;
    
    // 8081: WebSockets Data (RadarJSON)
    // 8080: Servidor HTTP Cliente (VanillaJS HTML)
    SentinelServer server(8081, 8080);
    
    std::cout << "[INFO] Sistema central de radar en linea." << std::endl;
    std::cout << "[INFO] Acceda a la web en: http://localhost:8080" << std::endl;
    std::cout << "[INFO] Cierre la ventana o Finalice la depuracion para detener." << std::endl;
    
    // Auto-Abrir el navegador por defecto en Windows (Magia Pura)
    system("start http://localhost:8080");
    
    // Qt Event Loop que mantiene vivas las conexiones WebSocket
    return a.exec();
}
