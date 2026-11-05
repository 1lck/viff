# IntelliJ Platform icons

The SVG files in this directory are copied unchanged from the IntelliJ
Platform (https://github.com/JetBrains/intellij-community), directories
`platform/icons/src/expui` and `platform/dvcs-impl/shared/resources/icons`.

Copyright 2000-2025 JetBrains s.r.o. and contributors.
Licensed under the Apache License, Version 2.0
(https://www.apache.org/licenses/LICENSE-2.0).

`go generate ./internal/ui/icons` compiles them into Go values with Viva's
icon compiler; the generated file is checked in.

## bot.svg, bot_dark.svg

These two draw the AI button for the commit message generator. They are not from
the IntelliJ set: they are a 16×16 redraw, made for this project, of a 24×24
robot outline supplied for the feature, using the stroke colours of the icons
around them. The project's own license applies; no JetBrains copyright is
claimed here.
