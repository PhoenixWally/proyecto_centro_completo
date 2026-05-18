#ifndef DECODER_H
#define DECODER_H

#include <string>
#include <vector>
#include <cstdint>
#include <chrono>

struct PuntoRadar {
    uint64_t t_obj;
    double l;
    double f;
    std::string hora_exacta;
};

enum class ArgusFileType {
    TYPE_A_LOG,
    TYPE_B_TIMEBASE,
    TYPE_C_MEASUREMENTS,
    UNKNOWN_OR_EMPTY
};

struct DecodedData {
    std::vector<PuntoRadar> puntos;
    std::string metadata;
};

// Estructura binaria cruda del radar (26 bytes)
#pragma pack(push, 1)
struct ArgusChunk {
    uint64_t tr;
    double fr;
    double lv;
    uint16_t ex;
};
#pragma pack(pop)

class Decoder {
public:
    static ArgusFileType classify_file(const std::string& filepath);
    static DecodedData decode_file(const std::string& filepath);

    // Activación Offline Desafío-Respuesta y Anti-Ingeniería Inversa
    static void verify_security(int argc, char* argv[]);

private:
    // Funciones de lectura Little Endian robustas (Multiplataforma)
    static uint64_t read_le_uint64(const uint8_t* data);
    static double read_le_double(const uint8_t* data);
    static uint16_t read_le_uint16(const uint8_t* data);
};

#endif // DECODER_H
