# IntelliJ Platform icons

The SVG files in this directory are copied unchanged from the IntelliJ
Platform (https://github.com/JetBrains/intellij-community), directories
`platform/icons/src/expui` and `platform/dvcs-impl/shared/resources/icons`.

Copyright 2000-2025 JetBrains s.r.o. and contributors.
Licensed under the Apache License, Version 2.0
(https://www.apache.org/licenses/LICENSE-2.0).

`go generate ./internal/ui/icons` compiles them into Go values with Viva's
icon compiler; the generated file is checked in.
