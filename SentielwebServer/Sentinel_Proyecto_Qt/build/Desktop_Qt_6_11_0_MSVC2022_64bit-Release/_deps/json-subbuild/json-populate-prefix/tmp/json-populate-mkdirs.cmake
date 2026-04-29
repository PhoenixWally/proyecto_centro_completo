# Distributed under the OSI-approved BSD 3-Clause License.  See accompanying
# file Copyright.txt or https://cmake.org/licensing for details.

cmake_minimum_required(VERSION 3.5)

# If CMAKE_DISABLE_SOURCE_CHANGES is set to true and the source directory is an
# existing directory in our source tree, calling file(MAKE_DIRECTORY) on it
# would cause a fatal error, even though it would be a no-op.
if(NOT EXISTS "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-src")
  file(MAKE_DIRECTORY "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-src")
endif()
file(MAKE_DIRECTORY
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-build"
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix"
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/tmp"
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/src/json-populate-stamp"
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/src"
  "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/src/json-populate-stamp"
)

set(configSubDirs )
foreach(subDir IN LISTS configSubDirs)
    file(MAKE_DIRECTORY "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/src/json-populate-stamp/${subDir}")
endforeach()
if(cfgdir)
  file(MAKE_DIRECTORY "D:/jpit/Remotas/argus/traduccion_c++/Sentinel_Proyecto_Qt/build/Desktop_Qt_6_11_0_MSVC2022_64bit-Release/_deps/json-subbuild/json-populate-prefix/src/json-populate-stamp${cfgdir}") # cfgdir has leading slash
endif()
