#include "VisorCapturas.h"
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QPixmap>
#include <filesystem>
#include <iostream>

VisorCapturas::VisorCapturas(const std::vector<std::string>& imagenes, const std::string& rutabase, QWidget *parent)
    : QDialog(parent), lista_imgs(imagenes), ruta_base(rutabase) {
    this->setWindowTitle("Visor de Capturas de Radar");
    this->resize(1200, 800);
    this->setStyleSheet("background-color: #0d0d0d; color: white;");

    // Implementación visual reducida
    // El main layout es vertical
    QVBoxLayout *main_layout = new QVBoxLayout(this);

    image_label = new QLabel("Cargando imagen...");
    image_label->setAlignment(Qt::AlignCenter);
    
    info_label = new QLabel("0 / 0 | Archivo: ...");
    info_label->setAlignment(Qt::AlignCenter);
    
    QHBoxLayout *nav_layout = new QHBoxLayout();
    QPushButton* btn_ant = new QPushButton("< Anterior");
    QPushButton* btn_sig = new QPushButton("Siguiente >");

    // Conectamos botones con SLOTS
    connect(btn_ant, &QPushButton::clicked, this, &VisorCapturas::anterior);
    connect(btn_sig, &QPushButton::clicked, this, &VisorCapturas::siguiente);

    nav_layout->addStretch();
    nav_layout->addWidget(btn_ant);
    nav_layout->addWidget(btn_sig);
    nav_layout->addStretch();

    main_layout->addWidget(image_label, 1);
    main_layout->addWidget(info_label);
    main_layout->addLayout(nav_layout);

    // Pintar primera imagen en pantalla si existe
    mostrar_imagen();
}

void VisorCapturas::anterior() {
    if (index_actual > 0) {
        index_actual--;
        mostrar_imagen();
    }
}

void VisorCapturas::siguiente() {
    if (index_actual < (int)lista_imgs.size() - 1) {
        index_actual++;
        mostrar_imagen();
    }
}

void VisorCapturas::mostrar_imagen() {
    if (lista_imgs.empty()) return;

    std::filesystem::path full_path = std::filesystem::path(ruta_base) / lista_imgs[index_actual];
    
    QPixmap pixmap(QString::fromStdString(full_path.string()));
    if (!pixmap.isNull()) {
        // Redimensionar automáticamente usando AspectRatio exacto ("contain" en CSS)
        image_label->setPixmap(pixmap.scaled(image_label->size(), Qt::KeepAspectRatio, Qt::SmoothTransformation));
    } else {
        image_label->setText("Imposible cargar el archivo QPixmap.");
    }
    
    info_label->setText(QString::fromStdString(std::to_string(index_actual + 1) + " / " + std::to_string(lista_imgs.size()) + "  |  " + lista_imgs[index_actual]));
}
