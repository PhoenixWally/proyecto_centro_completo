#ifndef CONFIGURADORFUENTES_H
#define CONFIGURADORFUENTES_H

#include <QDialog>
#include <QListWidget>
#include <QLineEdit>
#include <QPushButton>
#include <unordered_map>
#include <string>

class ConfiguradorFuentes : public QDialog {
    Q_OBJECT
public:
    explicit ConfiguradorFuentes(std::unordered_map<std::string, std::string>& ref_fuentes, QWidget* parent = nullptr);
    ~ConfiguradorFuentes() = default;

    bool se_modificaron_fuentes() const { return cambios_realizados; }

private slots:
    void btn_add_clicked();
    void btn_delete_clicked();
    void list_item_selected();

private:
    void refrescar_lista();

    std::unordered_map<std::string, std::string>& fuentes;
    bool cambios_realizados = false;

    QListWidget* list_fuentes;
    QLineEdit* un_nombre;
    QLineEdit* un_ruta;
    QPushButton* btn_folder;
    QPushButton* btn_add;
    QPushButton* btn_del;
};

#endif
