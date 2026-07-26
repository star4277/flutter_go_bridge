# The Flutter tooling requires that developers have a version of Visual Studio
# installed that includes CMake 3.14 or later. You should not increase this
# version, as doing so will cause the plugin to fail to compile for some
# customers of the plugin.
cmake_minimum_required(VERSION 3.14)

# Project-level configuration.
set(PROJECT_NAME "REPLACE_ME_GO_MOD_NAME")
project(${PROJECT_NAME} LANGUAGES CXX)

include("../gokit/cmake/gokit.cmake")
apply_gokit(${PROJECT_NAME} ../../../../../../REPLACE_ME_GO_MOD_DIR REPLACE_ME_GO_MOD_NAME "")

# List of absolute paths to libraries that should be bundled with the plugin.
# This list could contain prebuilt libraries, or libraries created by an
# external build triggered from this build file.
set(REPLACE_ME_GO_MOD_NAME_bundled_libraries
  "${${PROJECT_NAME}_gokit_lib}"
  PARENT_SCOPE
)
