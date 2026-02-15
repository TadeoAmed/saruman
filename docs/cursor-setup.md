# Guía de Configuración de Go en Cursor

## Requisitos Previos

Para utilizar todas las características del IDE con Go en Cursor, debes completar los siguientes pasos:

## 1. Instalar gopls (Servidor de Lenguaje Go)

Ejecuta el siguiente comando en tu terminal:

```bash
go install golang.org/x/tools/gopls@latest
```

**gopls** es el servidor de lenguaje oficial de Go que proporciona características esenciales del IDE como:
- Ir a Definición
- Autocompletado
- Diagnóstico de errores
- Refactorización

## 2. Instalar la Extensión de Go

1. Abre el panel de Extensiones en Cursor (`Ctrl+Shift+X`)
2. Busca "Go"
3. Instala la extensión oficial **golang.go** del "Go Team at Google"

Sin esta extensión, Cursor no puede conectarse a gopls y perderás todas las características avanzadas del IDE.

## 3. Reiniciar Cursor

Después de completar ambos pasos, reinicia Cursor para que los cambios surtan efecto.

## Uso

Una vez configurado, podrás utilizar `Ctrl+Click` para navegar a las definiciones de funciones y tipos en tu código Go.
