#include "decoder.h"
#include "obfuscator.h"
#include <iostream>
#include <string>
#include <vector>
#include <exception>

int main(int argc, char *argv[]) {
  // 1. Sistema de Control Criptográfico Activo (HWID + Licencia)
  Decoder::verify_security(argc, argv);

  // 2. Si no ha salido en verify_security, validamos que nos hayan pasado un archivo
  if (argc < 2) {
    std::cerr << OBF("Uso: sentinel_core <archivo_binario>") << std::endl;
    return 1;
  }

  std::string filepath = argv[1];

  // Ignorar argumentos de control si llegan aquí (ya procesados por verify_security)
  if (filepath == OBF("--phoenixcode") || filepath == OBF("--install")) {
    return 1;
  }

  try {
    // 3. Decodificar el archivo binario
    DecodedData data = Decoder::decode_file(filepath);

    // 4. Imprimir la salida en formato JSON estructurado (JSON Lines)
    // Cada línea de salida estándar contiene un único punto de radar decodificado,
    // lo que permite al subproceso lector (Go) transmitir los datos en tiempo real
    // línea por línea sin bloquearse.
    for (const auto &p : data.puntos) {
      // Limpiamos los saltos de línea de la hora exacta (<br> -> |)
      std::string t_str = p.hora_exacta;
      size_t br_pos = t_str.find("<br>");
      if (br_pos != std::string::npos) {
        t_str.replace(br_pos, 4, " | ");
      }
      
      std::cout << "{\"t\":" << p.t_obj 
                << ",\"f\":" << p.f 
                << ",\"l\":" << p.l 
                << ",\"time\":\"" << t_str << "\"}" << std::endl;
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
