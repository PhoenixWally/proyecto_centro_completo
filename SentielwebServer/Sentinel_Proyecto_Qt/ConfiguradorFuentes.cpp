#include "ConfiguradorFuentes.h"
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QGroupBox>
#include <QLabel>
#include <QFileDialog>
#include <QMessageBox>

ConfiguradorFuentes::ConfiguradorFuentes(std::unordered_map<std::string, std::string>& ref_fuentes, QWidget* parent)
    : QDialog(parent), fuentes(ref_fuentes) {
    this->setWindowTitle("Gestor de Antenas y Fuentes");
    this->resize(600, 450);
    this->setStyleSheet("background-color: #1a1a1a; color: white;");

    QVBoxLayout* main_lay = new QVBoxLayout(this);

    // Lista principal
    QGroupBox* gbLista = new QGroupBox("📡 Fuentes de Radar Registradas");
    gbLista->setStyleSheet("color: #00FF00; font-weight: bold; border: 1px solid #00FF00;");
    QVBoxLayout* layLista = new QVBoxLayout(gbLista);
    list_fuentes = new QListWidget();
    list_fuentes->setStyleSheet("background-color: #0d0d0d; color: white; font-family: Consolas; border: none; font-size: 13px;");
    layLista->addWidget(list_fuentes);
    
    btn_del = new QPushButton("🗑️ Eliminar Fuente Seleccionada");
    btn_del->setStyleSheet("background-color: #C0392B; padding: 5px; font-weight: bold; border-radius: 3px; color: white;");
    btn_del->setEnabled(false);
    layLista->addWidget(btn_del);
    main_lay->addWidget(gbLista);

    // Controles de Adición/Edición C++
    QGroupBox* gbAdd = new QGroupBox("Añadir / Sobrescribir Base de Datos");
    gbAdd->setStyleSheet("color: #00FF00; font-weight: bold; border: 1px solid gray;");
    QGridLayout* layAdd = new QGridLayout(gbAdd);
    
    QLabel* l_nom = new QLabel("Nombre Comercial:");
    l_nom->setStyleSheet("color: white; border: none; font-weight: normal;");
    layAdd->addWidget(l_nom, 0, 0);
    un_nombre = new QLineEdit();
    un_nombre->setStyleSheet("background-color: #333; color: white; border: 1px solid gray; padding: 2px;");
    layAdd->addWidget(un_nombre, 0, 1, 1, 2);

    QLabel* l_rut = new QLabel("Ruta Local / Red UNC:");
    l_rut->setStyleSheet("color: white; border: none; font-weight: normal;");
    layAdd->addWidget(l_rut, 1, 0);
    un_ruta = new QLineEdit();
    un_ruta->setStyleSheet("background-color: #333; color: white; border: 1px solid gray; padding: 2px;");
    layAdd->addWidget(un_ruta, 1, 1);
    
    btn_folder = new QPushButton("📂 Explorar...");
    btn_folder->setStyleSheet("background-color: #555; color: white; border: 1px solid #777;");
    layAdd->addWidget(btn_folder, 1, 2);

    btn_add = new QPushButton("💾 Añadir Configuración");
    btn_add->setStyleSheet("background-color: #27AE60; padding: 8px; font-weight: bold; color: white; border-radius: 3px;");
    layAdd->addWidget(btn_add, 2, 0, 1, 3);
    
    main_lay->addWidget(gbAdd);

    // Rellenamos lista inicial de puntero heredado
    refrescar_lista();

    // Conexiones Eventos
    connect(btn_folder, &QPushButton::clicked, this, [this]() {
        QString dir = QFileDialog::getExistingDirectory(this, "Selecciona monitor de Antena", "", QFileDialog::ShowDirsOnly);
        if(!dir.isEmpty()){
            un_ruta->setText(dir);
        }
    });

    connect(btn_add, &QPushButton::clicked, this, &ConfiguradorFuentes::btn_add_clicked);
    connect(btn_del, &QPushButton::clicked, this, &ConfiguradorFuentes::btn_delete_clicked);
    connect(list_fuentes, &QListWidget::itemSelectionChanged, this, &ConfiguradorFuentes::list_item_selected);
}

void ConfiguradorFuentes::refrescar_lista() {
    list_fuentes->clear();
    for(const auto& val : fuentes) {
        QString str = QString::fromStdString(val.first + "  |  " + val.second);
        list_fuentes->addItem(str);
    }
}

void ConfiguradorFuentes::list_item_selected() {
    if(list_fuentes->currentRow() >= 0) {
        btn_del->setEnabled(true);
        // Autocompleta para fácil edición (Replicando V3 UI)
        QString t = list_fuentes->currentItem()->text();
        QStringList partes = t.split("  |  ");
        if(partes.size() == 2) {
            un_nombre->setText(partes[0]);
            un_ruta->setText(partes[1]);
        }
    } else {
        btn_del->setEnabled(false);
    }
}

void ConfiguradorFuentes::btn_add_clicked() {
    if(un_nombre->text().isEmpty() || un_ruta->text().isEmpty()) {
        QMessageBox::warning(this, "Error", "El nombre y la ruta son obligatorios.");
        return;
    }
    // Sobrescribe o Añade de forma nativa en STD MAP
    fuentes[un_nombre->text().toStdString()] = un_ruta->text().toStdString();
    cambios_realizados = true;
    refrescar_lista();
    un_nombre->clear();
    un_ruta->clear();
}

void ConfiguradorFuentes::btn_delete_clicked() {
    if(list_fuentes->currentRow() >= 0) {
        QString t = list_fuentes->currentItem()->text();
        QString nombre = t.split("  |  ")[0];
        fuentes.erase(nombre.toStdString()); // Elimina del mapa STL C++ nativo
        cambios_realizados = true;
        refrescar_lista();
        un_nombre->clear();
        un_ruta->clear();
    }
}
