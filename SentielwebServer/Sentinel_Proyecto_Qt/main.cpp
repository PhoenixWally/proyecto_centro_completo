#include <QApplication>
#include "ArgusSentinel.h"

int main(int argc, char *argv[]) {
    // Inicializar la clase base de cualquier interfaz gráfica de Qt (Backend, Fuentes, Estilos)
    QApplication app(argc, argv);

    // Instanciar la V3 del escáner Argus Sentinel que hemos traducido
    ArgusSentinel window;
    
    // Mostrar en pantalla (equivalente al root.mainloop() de Tkinter / Python)
    window.show();

    // Mantener la retención ejecutándose infinita. Si un QDialog hijo o esta ventana devuelven 'exit', 
    // app.exec() rompe, se limpia la memoria del recolector de Qt, y el proceso muere devolviendo int(0).
    return app.exec();
}
