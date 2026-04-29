cmake_minimum_required(VERSION 3.16)

project(SentinelWebServer VERSION 3.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 6.2 COMPONENTS Core WebSockets REQUIRED)

# Nlohmann JSON API
include(FetchContent)
FetchContent_Declare(json URL https://github.com/nlohmann/json/releases/download/v3.11.3/json.tar.xz)
FetchContent_MakeAvailable(json)

# HTTP Server library is header-only (httplib.h)
# We add it as an include directory using the current source dir

set(PROJECT_SOURCES
    main.cpp
    RadarMonitor.cpp
    RadarMonitor.h
    SentinelServer.cpp
    SentinelServer.h
    ConfiguradorFuentes.cpp
    ConfiguradorFuentes.h
)

add_executable(SentinelWebServer ${PROJECT_SOURCES})

target_include_directories(SentinelWebServer PRIVATE ${CMAKE_SOURCE_DIR})

target_link_libraries(SentinelWebServer PRIVATE
    Qt6::Core
    Qt6::WebSockets
    nlohmann_json::nlohmann_json
    Ws2_32
    Crypt32
    Mpr
)
