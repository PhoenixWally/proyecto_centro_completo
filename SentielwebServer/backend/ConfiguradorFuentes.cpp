#include "ConfiguradorFuentes.h"
#include <fstream>
#include <iostream>
#include <algorithm>
#include <Windows.h>
#include <winnetwk.h>

#pragma comment(lib, "mpr.lib")

using json = nlohmann::json;
const std::string CONFIG_FILE = "config_fuentes.json";

std::vector<FuenteRadar> ConfiguradorFuentes::cargarFuentes() {
    std::vector<FuenteRadar> fuentes;
    std::ifstream file(CONFIG_FILE);
    if (file.is_open()) {
        try {
            json j;
            file >> j;
            for (const auto& item : j) {
                FuenteRadar f;
                f.id = item.value("id", "");
                f.path = item.value("path", "");
                f.user = item.value("user", "");
                f.password = item.value("password", "");
                fuentes.push_back(f);
            }
        } catch (...) {}
    } else {
        // Fallback default Si no existe
        FuenteRadar fUMA = {"UMA", "//192.168.29.71/argus_db/UMA", "", ""};
        FuenteRadar fMA = {"MA", "//192.168.29.71/argus_db/MA", "", ""};
        fuentes.push_back(fUMA);
        fuentes.push_back(fMA);
        guardarFuentes(fuentes);
    }
    return fuentes;
}

void ConfiguradorFuentes::guardarFuentes(const std::vector<FuenteRadar>& fuentes) {
    json j = json::array();
    for (const auto& f : fuentes) {
        j.push_back({
            {"id", f.id},
            {"path", f.path},
            {"user", f.user},
            {"password", f.password}
        });
    }
    std::ofstream file(CONFIG_FILE);
    if (file.is_open()) {
        file << j.dump(4);
    }
}

bool ConfiguradorFuentes::conectarUNC(const std::string& ruta, const std::string& user, const std::string& password) {
    if (ruta.empty()) return false;

    // Solo autenticar si es una ruta UNC (empieza por // o \\)
    if (ruta.rfind("//", 0) != 0 && ruta.rfind("\\\\", 0) != 0) {
        return true; 
    }
    
    // Normalizar a backslashes para el Kernel de Windows
    std::string norm_ruta = ruta;
    std::replace(norm_ruta.begin(), norm_ruta.end(), '/', '\\');

    // Extraer la ruta raíz del servidor compartido "\\192.168.xyz.abc\recurso"
    size_t third_slash = norm_ruta.find_first_of("\\", 2);
    size_t fourth_slash = std::string::npos;
    if (third_slash != std::string::npos) {
        fourth_slash = norm_ruta.find_first_of("\\", third_slash + 1);
    }
    
    std::string root_share = norm_ruta;
    if (fourth_slash != std::string::npos) {
        root_share = norm_ruta.substr(0, fourth_slash);
    }

    std::cout << "[Security] Autenticando red hacia: " << root_share << std::endl;

    NETRESOURCEA nr;
    ZeroMemory(&nr, sizeof(NETRESOURCEA));
    nr.dwType = RESOURCETYPE_DISK;
    nr.lpRemoteName = const_cast<char*>(root_share.c_str());

    DWORD res = WNetAddConnection2A(&nr, 
                                   password.empty() ? nullptr : password.c_str(), 
                                   user.empty() ? nullptr : user.c_str(), 
                                   CONNECT_TEMPORARY);
                                   
    if (res == NO_ERROR) {
        std::cout << "[Security] Identificacion exitosa en UNC." << std::endl;
        return true;
    } else if (res == 1219) { // ERROR_SESSION_CREDENTIAL_CONFLICT
        std::cout << "[Security] Ya existe una credencial activa previa." << std::endl;
        return true;
    } else if (res == 85) { // ERROR_ALREADY_ASSIGNED
        return true;
    }

    std::cerr << "[Security] Bloqueo o Fallo de red hacia " << root_share << " (WINAPI Error Código: " << res << ")\n";
    return false;
}
