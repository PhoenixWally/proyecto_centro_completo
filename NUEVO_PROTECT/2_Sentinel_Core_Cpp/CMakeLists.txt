cmake_minimum_required(VERSION 3.14)
project(Sentinel_Decoder_Core)

# Requerir C++17 (Vital para los tipos de datos y cronos)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(APP_ICON "${CMAKE_CURRENT_SOURCE_DIR}/phoenix_icon.ico" CACHE FILEPATH "Application icon")
set(CMAKE_AUTORCC ON)
configure_file(
    "${CMAKE_CURRENT_SOURCE_DIR}/appicon.rc.in"
    "${CMAKE_CURRENT_BINARY_DIR}/appicon.rc"
    @ONLY
)

# Compilar como ejecutable minimalista de decodificación
add_executable(sentinel_core 
    decoder.h
    obfuscator.h
    decoder.cpp
    main.cpp 
    "${CMAKE_CURRENT_BINARY_DIR}/appicon.rc"
    "${CMAKE_CURRENT_SOURCE_DIR}/resources.qrc"
)

target_include_directories(sentinel_core PUBLIC 
    ${CMAKE_CURRENT_SOURCE_DIR}
)

# =========================================================
# BLINDAJE DE COMPILADOR (MSVC STRIPPING & OBFUSCATION)
# =========================================================
if(MSVC)
    # Desactiva la generación de información de depuración (elimina PDBs)
    string(REPLACE "/Zi" "" CMAKE_CXX_FLAGS_RELEASE "${CMAKE_CXX_FLAGS_RELEASE}")
    string(REPLACE "/ZI" "" CMAKE_CXX_FLAGS_RELEASE "${CMAKE_CXX_FLAGS_RELEASE}")
    string(REPLACE "/DEBUG" "" CMAKE_EXE_LINKER_FLAGS_RELEASE "${CMAKE_EXE_LINKER_FLAGS_RELEASE}")
    
    # Flags de ofuscación nivel compilador
    target_compile_options(sentinel_core PRIVATE 
        /Ob2      # Expansión agresiva de funciones inline (rompe el flujo lineal para el debugger)
        /Os       # Favorecer tamaño de código (empaqueta mejor las instrucciones)
        /GL       # Optimización de todo el programa
    )
    
    target_link_options(sentinel_core PRIVATE 
        /EMITPDB:NO     # Prohíbe terminantemente crear archivos de símbolos
        /LTCG           # Generación de código en tiempo de enlace
        /DYNAMICBASE    # ASLR: Aleatoriza la posición en memoria RAM cada vez que se ejecuta
    )
endif()

# Enlazar la librería advapi32 en Windows (necesaria para leer el registro MachineGuid)
if(WIN32)
    target_link_libraries(sentinel_core advapi32)
endif()
