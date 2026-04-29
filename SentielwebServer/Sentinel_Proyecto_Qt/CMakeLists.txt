cmake_minimum_required(VERSION 3.16)

project(ArgusSentinel VERSION 1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Habilitar automoc, autouic y autorcc para Qt (obligatorio para Signals/Slots)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

# Encontrar módulos principales de Qt6
find_package(Qt6 REQUIRED COMPONENTS Core Gui Widgets DataVisualization PrintSupport)

# Integración externa: nlohmann_json desde GitHub directo usando FetchContent.
# Esto nos salva de tener que descargar JSON para C++ manualmente. CMake lo arrastra y enlaza automáticamente.
include(FetchContent)
FetchContent_Declare(json
    GIT_REPOSITORY https://github.com/nlohmann/json.git
    GIT_TAG v3.11.3
)
FetchContent_MakeAvailable(json)

# Archivos fuente del programa
set(PROJECT_SOURCES
    main.cpp
    ArgusSentinel.cpp
    ArgusSentinel.h
    VisorCapturas.cpp
    VisorCapturas.h
    ConfiguradorFuentes.cpp
    ConfiguradorFuentes.h
    qcustomplot.cpp
    qcustomplot.h
)

# Compilar ejecutable (WIN32 evita abrir consola CMD negra genérica en Windows en fase de "Release" de la app visual)
add_executable(ArgusSentinel WIN32 ${PROJECT_SOURCES})

# Enlazar las librerías al ejecutable (Core de Qt y de parsing JSON de nlohmann)
target_link_libraries(ArgusSentinel PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Widgets
    Qt6::DataVisualization
    Qt6::PrintSupport
    nlohmann_json::nlohmann_json
)
