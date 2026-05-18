#include "decoder.h"
#include "obfuscator.h"
#include <algorithm>
#include <cstdlib>
#include <cstring>
#include <ctime>
#include <fstream>
#include <iostream>

#ifdef _WIN32
#define NOMINMAX
#include <intrin.h>
#include <windows.h>
#else
#if defined(__x86_64__) || defined(__i386__)
#include <x86intrin.h>
#else
inline unsigned long long __rdtsc() { return 0; }
#endif
#endif

// --- Seguridad y Anti-Ingeniería Inversa ---
namespace SentinelSecurity {
const std::string SECRET_SALT = OBF("PHOENIX_RADAR_26");
uint64_t active_filetime_const = 0;

#define ROTRIGHT(word, bits) (((word) >> (bits)) | ((word) << (32 - (bits))))
#define CH(x, y, z) (((x) & (y)) ^ (~(x) & (z)))
#define MAJ(x, y, z) (((x) & (y)) ^ ((x) & (z)) ^ ((y) & (z)))
#define EP0(x) (ROTRIGHT(x, 2) ^ ROTRIGHT(x, 13) ^ ROTRIGHT(x, 22))
#define EP1(x) (ROTRIGHT(x, 6) ^ ROTRIGHT(x, 11) ^ ROTRIGHT(x, 25))
#define SIG0(x) (ROTRIGHT(x, 7) ^ ROTRIGHT(x, 18) ^ ((x) >> 3))
#define SIG1(x) (ROTRIGHT(x, 17) ^ ROTRIGHT(x, 19) ^ ((x) >> 10))

std::string sha256_hash(const std::string &input) {
  std::vector<uint8_t> data(input.begin(), input.end());
  uint64_t bitlen = data.size() * 8;
  data.push_back(0x80);
  while ((data.size() % 64) != 56) {
    data.push_back(0x00);
  }
  for (int i = 7; i >= 0; --i) {
    data.push_back(static_cast<uint8_t>((bitlen >> (i * 8)) & 0xFF));
  }

  uint32_t state[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
                       0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};

  const uint32_t k[64] = {
      0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1,
      0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
      0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786,
      0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
      0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147,
      0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
      0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
      0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
      0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a,
      0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
      0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2};

  for (size_t i = 0; i < data.size(); i += 64) {
    uint32_t m[64];
    for (int j = 0, p = 0; j < 16; ++j, p += 4) {
      m[j] = (data[i + p] << 24) | (data[i + p + 1] << 16) |
             (data[i + p + 2] << 8) | (data[i + p + 3]);
    }
    for (int j = 16; j < 64; ++j) {
      m[j] = SIG1(m[j - 2]) + m[j - 7] + SIG0(m[j - 15]) + m[j - 16];
    }

    uint32_t a = state[0], b = state[1], c = state[2], d = state[3],
             e = state[4], f = state[5], g = state[6], h = state[7];

    for (int j = 0; j < 64; ++j) {
      uint32_t t1 = h + EP1(e) + CH(e, f, g) + k[j] + m[j];
      uint32_t t2 = EP0(a) + MAJ(a, b, c);
      h = g;
      g = f;
      f = e;
      e = d + t1;
      d = c;
      c = b;
      b = a;
      a = t1 + t2;
    }

    state[0] += a;
    state[1] += b;
    state[2] += c;
    state[3] += d;
    state[4] += e;
    state[5] += f;
    state[6] += g;
    state[7] += h;
  }

  char hex[65];
  snprintf(hex, sizeof(hex), OBF("%08x%08x%08x%08x%08x%08x%08x%08x").c_str(), state[0],
           state[1], state[2], state[3], state[4], state[5], state[6],
           state[7]);
  return std::string(hex);
}

std::string get_machine_hwid() {
  std::string hwid = OBF("UNKNOWN_MACHINE_ID");
#ifdef _WIN32
  HKEY hKey;
  if (RegOpenKeyExA(HKEY_LOCAL_MACHINE, OBF("SOFTWARE\\Microsoft\\Cryptography").c_str(), 0,
                    KEY_READ | KEY_WOW64_64KEY, &hKey) == ERROR_SUCCESS) {
    char guid[256];
    DWORD size = sizeof(guid);
    if (RegQueryValueExA(hKey, OBF("MachineGuid").c_str(), nullptr, nullptr, (LPBYTE)guid,
                         &size) == ERROR_SUCCESS) {
      hwid = guid;
    }
    RegCloseKey(hKey);
  }
#else
  std::ifstream file(OBF("/etc/machine-id"));
  if (file.is_open()) {
    std::getline(file, hwid);
  }
#endif
  return hwid;
}

uint32_t light_hash(const std::string &str) {
  uint32_t hash = 5381;
  for (char c : str) {
    hash = ((hash << 5) + hash) + c;
  }
  return hash;
}

std::string obfuscate(const std::string &data) {
  std::string res = data;
  for (size_t i = 0; i < res.length(); ++i) {
    res[i] ^= SECRET_SALT[i % SECRET_SALT.length()];
  }
  return res;
}

uint64_t fnv1a_64(const std::string &text) {
  uint64_t hash = 14695981039346656037ULL;
  for (char c : text) {
    hash ^= static_cast<uint64_t>(c);
    hash *= 1099511628211ULL;
  }
  return hash;
}
} // namespace SentinelSecurity

void Decoder::verify_security(int argc, char *argv[]) {
  std::string hwid =
      SentinelSecurity::get_machine_hwid(); // Usamos MachineGuid/UUID nativo
                                            // para mayor compatibilidad
                                            // estática

  // Fase 0 y 1: CLI Configuration
  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];

    // 1. Calculamos el HWID Público (El que tú ves y tecleas en Go)
    std::string public_hwid =
        OBF("HWID-") + SentinelSecurity::sha256_hash(hwid).substr(0, 8);

    if (arg == OBF("--phoenixcode")) {
      std::cout << public_hwid << std::endl;
      exit(0);
    } else if (arg == OBF("--install")) {
      if (i + 1 < argc) {
        std::string user_token = argv[i + 1];

        // 2. Replicamos la matemática exacta de Go ( HWID + "||" + SECRET_SALT )
        std::string data_to_hash =
            public_hwid + OBF("||") + SentinelSecurity::SECRET_SALT;
        std::string full_expected = SentinelSecurity::sha256_hash(data_to_hash);

        // 3. Recortamos a 16 caracteres
        std::string expected_token = full_expected.substr(0, 16);

        // 4. Forzamos a MAYÚSCULAS para que coincida milimétricamente con Go
        for (char &c : expected_token) {
          c = toupper(c);
        }

        if (user_token == expected_token) {
          std::string obf_hwid = SentinelSecurity::obfuscate(hwid);
          std::ofstream lockfile(OBF(".sentinel.lock"), std::ios::binary);
          if (lockfile.is_open()) {
            lockfile.write(obf_hwid.c_str(), obf_hwid.length());
            lockfile.close();
            std::cout << OBF("Binding completado") << std::endl;
            exit(0);
          }
        }
        // Si el token es erróneo, muere en silencio
        exit(1);
      } else {
        std::cerr << OBF("Error: Falta el token de instalacion. Uso: sentinel_core --install <TOKEN>") << std::endl;
        exit(1);
      }
    }
  }

  // Fase 2: Control Criptográfico Activo (Modo Ejecución)
  std::ifstream lockfile(OBF(".sentinel.lock"), std::ios::binary);
  std::string obf_hwid;
  if (lockfile.is_open()) {
    obf_hwid.assign((std::istreambuf_iterator<char>(lockfile)),
                    std::istreambuf_iterator<char>());
  }

  uint64_t t_start = __rdtsc();

  std::string deobf_hwid_lock = SentinelSecurity::obfuscate(obf_hwid);

  uint64_t key_actual = SentinelSecurity::fnv1a_64(hwid);
  uint64_t key_lock = SentinelSecurity::fnv1a_64(deobf_hwid_lock);

  // Derivar llave criptográfica (Si coinciden, el XOR será 0)
  uint64_t derived_key = key_actual ^ key_lock;

  uint64_t t_end = __rdtsc();
  uint64_t t_diff = t_end - t_start;

  // Fase 3: Mina Anti-Debugger sin ramas condicionales
  // Mutación matemática pura: si t_diff excede el límite, la booleana se evalúa
  // como 1 y muta la llave
  derived_key ^= ((t_diff > 1000000000ULL) * 0xDEADBEEF);

  // Ocultación Matemática Constante FILETIME (116444736000000000ULL)
  // XOR Precalculado de (116444736000000000ULL ^ SECRET_MASK)
  const uint64_t OBFUSCATED_FILETIME = 6613513532390824538ULL;
  const uint64_t SECRET_MASK = 6510615555426900570ULL;

  SentinelSecurity::active_filetime_const =
      OBFUSCATED_FILETIME ^ SECRET_MASK ^ derived_key;
}

// --- Funciones Little Endian ---
uint64_t Decoder::read_le_uint64(const uint8_t *data) {
  return static_cast<uint64_t>(data[0]) |
         (static_cast<uint64_t>(data[1]) << 8) |
         (static_cast<uint64_t>(data[2]) << 16) |
         (static_cast<uint64_t>(data[3]) << 24) |
         (static_cast<uint64_t>(data[4]) << 32) |
         (static_cast<uint64_t>(data[5]) << 40) |
         (static_cast<uint64_t>(data[6]) << 48) |
         (static_cast<uint64_t>(data[7]) << 56);
}

double Decoder::read_le_double(const uint8_t *data) {
  uint64_t val = read_le_uint64(data);
  double d;
  std::memcpy(&d, &val, sizeof(double));
  return d;
}

uint16_t Decoder::read_le_uint16(const uint8_t *data) {
  return static_cast<uint16_t>(data[0]) | (static_cast<uint16_t>(data[1]) << 8);
}

// --- Clasificación de Archivos (Cabeceras) ---
ArgusFileType Decoder::classify_file(const std::string &filepath) {
  std::ifstream file(filepath, std::ios::binary);
  if (!file.is_open())
    return ArgusFileType::UNKNOWN_OR_EMPTY;

  std::vector<uint8_t> head(256, 0);
  file.read(reinterpret_cast<char *>(head.data()), head.size());
  std::streamsize bytes_read = file.gcount();
  if (bytes_read == 0)
    return ArgusFileType::UNKNOWN_OR_EMPTY;

  std::string ascii_equiv;
  for (int i = 0; i < bytes_read; i++) {
    if (head[i] != 0x00 && head[i] >= 32 && head[i] <= 126) {
      ascii_equiv += static_cast<char>(head[i]);
    }
  }

  if (ascii_equiv.find(OBF("CTER")) != std::string::npos ||
      ascii_equiv.find(OBF("ALARMAS")) != std::string::npos ||
      ascii_equiv.find(OBF("EA-MALAGA")) != std::string::npos ||
      ascii_equiv.find(OBF("Log")) != std::string::npos) {
    return ArgusFileType::TYPE_A_LOG;
  }

  int null_count = 0;
  int check_len = std::min(static_cast<int>(bytes_read), 176);
  for (int i = 0; i < check_len; i++) {
    if (head[i] == 0x00)
      null_count++;
  }

  if (check_len >= 170 && null_count > 150) {
    return ArgusFileType::TYPE_B_TIMEBASE;
  }

  return ArgusFileType::TYPE_C_MEASUREMENTS;
}

// --- Decodificación de Puntos y Footer UTF-16 ---
DecodedData Decoder::decode_file(const std::string &filepath) {
  DecodedData result;
  std::ifstream file(filepath, std::ios::binary | std::ios::ate);
  if (!file.is_open())
    return result;

  std::streamsize sz = file.tellg();
  if (sz < 28)
    return result;

  std::streamsize offset = sz - 2;
  if (offset > 200000 * 26)
    offset = 200000 * 26;
  std::streamsize start_pos = sz - offset;

  std::streamsize residuo = (start_pos - 2) % 26;
  if (residuo != 0)
    start_pos -= residuo;
  if (start_pos < 2)
    start_pos = 2;

  file.seekg(start_pos, std::ios::beg);
  std::vector<uint8_t> buffer(sz - start_pos);
  if (!file.read(reinterpret_cast<char *>(buffer.data()), buffer.size()))
    return result;

  size_t block_offset = 0;
  while (block_offset + 26 <= buffer.size()) {
    const uint8_t *chunk_data = buffer.data() + block_offset;

    // Extracción explícita Little Endian
    uint64_t tr = read_le_uint64(chunk_data);
    double fr = read_le_double(chunk_data + 8);
    double lv = read_le_double(chunk_data + 16);

    block_offset += 26;

    uint64_t total_sec = (tr / 10000000ULL);
    uint64_t const_sec = SentinelSecurity::active_filetime_const / 10000000ULL;

    // Ejecución ciega basada en el estado criptográfico de la constante
    time_t unix_time = static_cast<time_t>(total_sec - const_sec);

    // Validación de seguridad para evitar crashes fatales de gmtime_s/localtime_s con timestamps corruptos del footer
    if (unix_time < 1577836800LL || unix_time > 2524608000LL) {
      continue;
    }

    struct tm parts_utc = {0}, parts_loc = {0};

#ifdef _WIN32
    gmtime_s(&parts_utc, &unix_time);
    localtime_s(&parts_loc, &unix_time);
#else
    gmtime_r(&unix_time, &parts_utc);
    localtime_r(&unix_time, &parts_loc);
#endif

    PuntoRadar pr;
    pr.f = fr / 1000000.0;

    if (pr.f > 10.0 && pr.f < 50000.0) {
      pr.l = lv;
      pr.t_obj = static_cast<uint64_t>(unix_time);

      char bUtc[32], bLoc[32];
      std::strftime(bUtc, sizeof(bUtc), OBF("UTC %H:%M:%S").c_str(), &parts_utc);
      std::strftime(bLoc, sizeof(bLoc), OBF("LOC %H:%M:%S").c_str(), &parts_loc);
      pr.hora_exacta = std::string(bUtc) + OBF("<br>") + std::string(bLoc);

      result.puntos.push_back(pr);
    }
  }

  // Extracción de Footer UTF-16 Metadatos
  std::streamsize tail_offset = std::max<std::streamsize>(0, sz - 4096);
  file.seekg(tail_offset, std::ios::beg);
  std::vector<uint8_t> tail_buf(sz - tail_offset);
  file.read(reinterpret_cast<char *>(tail_buf.data()), tail_buf.size());

  std::string tail_ascii = "";
  int valid_chars = 0;
  for (size_t i = 0; i < tail_buf.size(); i++) {
    char c = static_cast<char>(tail_buf[i]);
    if (c >= 32 && c <= 126) {
      tail_ascii += c;
      valid_chars++;
    } else if (tail_buf[i] == 0x00) {
      // Ignorar el null byte (padding de UTF-16)
    } else {
      if (!tail_ascii.empty() && tail_ascii.back() != ' ')
        tail_ascii += ' ';
    }
  }

  if (valid_chars > 20 && (tail_ascii.find(OBF("202")) != std::string::npos ||
                           tail_ascii.find(OBF("Hz")) != std::string::npos ||
                           tail_ascii.find(OBF("CTER")) != std::string::npos ||
                           tail_ascii.find(OBF("Modo")) != std::string::npos)) {
    std::string footer_clean = "";
    for (char c : tail_ascii) {
      if (c == ' ' && !footer_clean.empty() && footer_clean.back() == ' ')
        continue;
      footer_clean += c;
    }
    if (footer_clean.length() > 5) {
      result.metadata = footer_clean;
    }
  }

  return result;
}
