#ifndef OBFUSCATOR_H
#define OBFUSCATOR_H

#include <string>

namespace SentinelObf {
    // Generate a simple compile-time seed using __TIME__
    constexpr char seed() {
        return (__TIME__[7] ^ __TIME__[6] ^ __TIME__[4] ^ __TIME__[3] ^ __TIME__[1] ^ __TIME__[0]) | 0x33;
    }

    template <size_t N>
    struct ObfuscatedString {
        char data[N];
        char key;

        constexpr ObfuscatedString(const char* str) : data{}, key(seed() == 0 ? 0xAA : seed()) {
            for (size_t i = 0; i < N; ++i) {
                data[i] = str[i] ^ key;
            }
        }

        // Decrypt automatically at runtime
        std::string decrypt() const {
            std::string res;
            res.reserve(N - 1);
            for (size_t i = 0; i < N - 1; ++i) {
                res.push_back(data[i] ^ key);
            }
            return res;
        }
    };
}

#define OBF(str) (SentinelObf::ObfuscatedString<sizeof(str)>(str).decrypt())

#endif // OBFUSCATOR_H
