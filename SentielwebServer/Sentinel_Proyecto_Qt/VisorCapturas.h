#ifndef VISORCAPTURAS_H
#define VISORCAPTURAS_H

#include <QDialog>
#include <QLabel>
#include <QPushButton>
#include <vector>
#include <string>

class VisorCapturas : public QDialog {
    Q_OBJECT
public:
    explicit VisorCapturas(const std::vector<std::string>& imagenes, const std::string& rutabase, QWidget *parent = nullptr);
    ~VisorCapturas() = default;

private slots:
    void siguiente();
    void anterior();

private:
    void mostrar_imagen();

    std::vector<std::string> lista_imgs;
    std::string ruta_base;
    int index_actual = 0;

    QLabel* image_label;
    QLabel* info_label;
};

#endif // VISORCAPTURAS_H
