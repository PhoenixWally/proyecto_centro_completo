#ifndef CONFIGURADOR_FUENTES_H
#define CONFIGURADOR_FUENTES_H

#include <string>
#include <vector>
#include <nlohmann/json.hpp>

struct FuenteRadar {
    std::string id;
    std::string path;
    std::string user;
    std::string password;
};

class ConfiguradorFuentes {
public:
    static std::vector<FuenteRadar> cargarFuentes();
    static void guardarFuentes(const std::vector<FuenteRadar>& fuentes);
    
    // Función de red Windows
    static bool conectarUNC(const std::string& ruta, const std::string& user, const std::string& password);
};

#endif
