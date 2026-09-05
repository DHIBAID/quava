cmake_minimum_required(VERSION 3.16)

project(quava VERSION 0.1 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Qt6 REQUIRED COMPONENTS Quick Network)

qt_standard_project_setup(REQUIRES 6.10)

# Content of downloaded design from Figma to Qt or similar

if (EXISTS "${CMAKE_CURRENT_LIST_DIR}/importedcontent/CMakeLists.txt")
add_subdirectory(${CMAKE_CURRENT_LIST_DIR}/importedcontent)
endif()

qt_add_executable(
    appquava
    main.cpp
    mdns_advertiser.cpp
    mdns_advertiser.h
    quava_tcp_server.cpp
    quava_tcp_server.h
)

qt_add_qml_module(
    appquava
    URI quava
    QML_FILES
    Main.qml
)

# Qt for iOS sets MACOSX_BUNDLE_GUI_IDENTIFIER automatically since Qt 6.1.
# If you are developing for iOS or macOS you should consider setting an
# explicit, fixed bundle identifier manually though.

set_target_properties(
    appquava PROPERTIES

    MACOSX_BUNDLE_GUI_IDENTIFIER my.example.com
    MACOSX_BUNDLE_BUNDLE_VERSION ${PROJECT_VERSION}
    MACOSX_BUNDLE_SHORT_VERSION_STRING ${PROJECT_VERSION_MAJOR}.${PROJECT_VERSION_MINOR}
    MACOSX_BUNDLE TRUE
    WIN32_EXECUTABLE TRUE
)

target_link_libraries(
    appquava
    PRIVATE
    Qt6::Quick
    Qt6::Network
)

# Android OpenSSL
#
# Qt 6.10 provides the OpenSSL TLS backend:
# plugins/tls/libplugins_tls_qopensslbackend_<ABI>.so
#
# The Android OpenSSL package provides the actual OpenSSL libraries.

if(ANDROID)
    set(ANDROID_OPENSSL_ROOT "$ENV{HOME}/Android/Sdk/android_openssl")

    if(ANDROID_ABI STREQUAL "arm64-v8a")
        set(OPENSSL_ANDROID_LIB_DIR
            "${ANDROID_OPENSSL_ROOT}/ssl_3/arm64-v8a")
    elseif(ANDROID_ABI STREQUAL "armeabi-v7a")
        set(OPENSSL_ANDROID_LIB_DIR
            "${ANDROID_OPENSSL_ROOT}/ssl_3/armeabi-v7a")
    elseif(ANDROID_ABI STREQUAL "x86_64")
        set(OPENSSL_ANDROID_LIB_DIR
            "${ANDROID_OPENSSL_ROOT}/ssl_3/x86_64")
    elseif(ANDROID_ABI STREQUAL "x86")
        set(OPENSSL_ANDROID_LIB_DIR
            "${ANDROID_OPENSSL_ROOT}/ssl_3/x86")
    else()
        message(FATAL_ERROR
            "Unsupported Android ABI for OpenSSL: ${ANDROID_ABI}")
    endif()

    set_target_properties(appquava PROPERTIES
        QT_ANDROID_EXTRA_LIBS
            "${OPENSSL_ANDROID_LIB_DIR}/libcrypto_3.so;${OPENSSL_ANDROID_LIB_DIR}/libssl_3.so"
    )
endif()

set_property(
    TARGET appquava PROPERTY
    QT_ANDROID_PACKAGE_SOURCE_DIR
    ${CMAKE_CURRENT_SOURCE_DIR}/android
)

install(
    TARGETS appquava
    BUNDLE DESTINATION .
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
    RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
)
