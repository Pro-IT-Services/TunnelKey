# Builds the OpenVPN 3 core (client) together with a small hand-written JNI
# bridge (ovpn3_jni.cpp). All third-party sources are downloaded at configure
# time and pinned by SHA-256, so no git submodules or SWIG are needed.

cmake_minimum_required(VERSION 3.22.1)
project(tunnelkey_ovpn3 C CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Reproducible builds
add_link_options("LINKER:--build-id=none")

include(FetchContent)
if(POLICY CMP0169)
    cmake_policy(SET CMP0169 OLD) # allow FetchContent_Populate for source-only deps
endif()
if(POLICY CMP0135)
    cmake_policy(SET CMP0135 NEW)
endif()

set(OPENVPN3_VERSION "3.11.7")

FetchContent_Declare(openvpn3
    URL https://github.com/OpenVPN/openvpn3/archive/refs/tags/release/${OPENVPN3_VERSION}.tar.gz
    URL_HASH SHA256=6ec04d7e7824f043ee2ec6c2d98ce2655350e5cdcc969ab60a5f68cecb232289)
FetchContent_Declare(asio
    URL https://github.com/chriskohlhoff/asio/archive/refs/tags/asio-1-38-2.tar.gz
    URL_HASH SHA256=9f2648fa483e58a6bf848d970ee0ea650ca19ed7769dfa520ed4f7b8d27af1db)
FetchContent_Declare(fmt
    URL https://github.com/fmtlib/fmt/releases/download/12.2.0/fmt-12.2.0.zip
    URL_HASH SHA256=a2f4a8d51178f954e4c339007f77edd76ba0cb2e36f87a48e5a5403d9be5878f)
FetchContent_Declare(lz4
    URL https://github.com/lz4/lz4/archive/refs/tags/v1.10.0.tar.gz
    URL_HASH SHA256=537512904744b35e232912055ccf8ec66d768639ff3abe5788d90d792ec5f48b)
FetchContent_Declare(mbedtls
    URL https://github.com/Mbed-TLS/mbedtls/releases/download/mbedtls-3.6.7/mbedtls-3.6.7.tar.bz2
    URL_HASH SHA256=a7e8bcbec0e6f761b4af24f25677626b35f762f68eef79c08677a363212d11f6)

# Source-only dependencies: download, but do not run their CMake projects.
foreach(dep openvpn3 asio fmt lz4)
    FetchContent_GetProperties(${dep})
    if(NOT ${dep}_POPULATED)
        FetchContent_Populate(${dep})
    endif()
endforeach()

# mbed TLS: static libraries, release tarball ships pre-generated sources.
set(ENABLE_PROGRAMS OFF CACHE BOOL "" FORCE)
set(ENABLE_TESTING OFF CACHE BOOL "" FORCE)
set(GEN_FILES OFF CACHE BOOL "" FORCE)
set(USE_SHARED_MBEDTLS_LIBRARY OFF CACHE BOOL "" FORCE)
set(USE_STATIC_MBEDTLS_LIBRARY ON CACHE BOOL "" FORCE)
set(MBEDTLS_FATAL_WARNINGS OFF CACHE BOOL "" FORCE)
# mbed TLS >= 3.6.3 refuses to verify a server certificate unless a hostname
# was set. OpenVPN never does hostname checks: the server is authenticated
# against the profile's CA (plus remote-cert-tls / verify-x509-name), exactly
# as with OpenVPN 2. Restore the pre-3.6.3 behaviour the OpenVPN 3 core expects.
add_compile_definitions(MBEDTLS_SSL_CLI_ALLOW_WEAK_CERTIFICATE_VERIFICATION_WITHOUT_HOSTNAME)
FetchContent_MakeAvailable(mbedtls)

add_library(lz4_static STATIC ${lz4_SOURCE_DIR}/lib/lz4.c)
target_include_directories(lz4_static PUBLIC ${lz4_SOURCE_DIR}/lib)

add_library(tunnelkey_ovpn3 SHARED
    ovpn3_jni.cpp
    ${openvpn3_SOURCE_DIR}/client/ovpncli.cpp
    ${openvpn3_SOURCE_DIR}/openvpn/crypto/data_epoch.cpp)

target_include_directories(tunnelkey_ovpn3 PRIVATE
    ${openvpn3_SOURCE_DIR}
    ${openvpn3_SOURCE_DIR}/client
    ${asio_SOURCE_DIR}/include
    ${fmt_SOURCE_DIR}/include)

target_compile_definitions(tunnelkey_ovpn3 PRIVATE
    USE_MBEDTLS
    USE_ASIO
    ASIO_STANDALONE
    HAVE_LZ4
    FMT_HEADER_ONLY
    OPENVPN_CORE_GIT_VERSION="release/${OPENVPN3_VERSION}")

target_compile_options(tunnelkey_ovpn3 PRIVATE -Wno-deprecated-declarations)

target_link_libraries(tunnelkey_ovpn3 PRIVATE
    mbedtls mbedx509 mbedcrypto lz4_static log)
