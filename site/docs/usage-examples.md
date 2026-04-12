# Ejemplos de uso

Ejemplos prácticos de cómo interactuar con Gitlab MCP en lenguaje natural. Simplemente escribe lo que necesitas y el asistente de IA elegirá la herramienta adecuada.

---

## Proyectos

```text
> Lista mis proyectos
> ¿Cuántos proyectos hay en el grupo "backend"?
> Crea un proyecto llamado "new-service" en el grupo "infra"
> Archiva el proyecto "legacy-app"
> Muéstrame los contribuidores de my-app
```

## Repositorio y archivos

```text
> Muéstrame los archivos en la raíz de my-app
> Muéstrame el contenido de src/main.go
> Compara la rama develop con main
> ¿Qué commits se han hecho esta semana?
```

## Merge Requests

```text
> Lista los MR abiertos del proyecto my-app
> Muéstrame los cambios del MR !15
> Crea un MR de feature-login a develop
> ¿Quién aprobó el MR !23?
> Haz merge del MR !15 con squash
> Resume los comentarios de revisión del MR !15
```

## Issues

```text
> Lista los bugs abiertos asignados a mí
> Crea un issue "Optimizar queries SQL" con etiqueta "performance"
> Cierra el issue #42 con un comentario
> ¿Cuántos issues hay en el milestone "v2.0"?
> Resume el issue #18
```

## CI/CD

```text
> ¿Cuál es el estado del último pipeline?
> ¿Por qué falló el pipeline #123?
> Muéstrame el log del job de tests
> Reintenta el pipeline fallido
> Lanza un nuevo pipeline en la rama develop
> Analiza la configuración CI del proyecto
```

## Análisis con IA

```text
> Analiza los cambios del MR !15
> Haz una revisión de seguridad del MR !23
> ¿Qué deuda técnica tiene el proyecto my-app?
> Genera notas de release de v1.0 a v2.0
> Genera un informe del milestone "v2.0"
> Analiza el historial de despliegues en producción
> Evalúa el alcance del issue #18
```

## Búsqueda

```text
> Busca "TODO" en el proyecto my-app
> Busca issues con etiqueta "critical" en el grupo "backend"
> Busca merge requests del autor "jdoe" en el grupo "platform"
```

## Grupos y usuarios

```text
> ¿Quién soy en GitLab?
> Lista los miembros del grupo "platform"
> ¿Qué actividad tuvo el usuario jdoe esta semana?
```

## Runners y entornos

```text
> Lista los runners disponibles
> ¿Cuál es el estado del entorno de producción?
> Lista los despliegues recientes de my-app
```

---

!!! tip "Consejos"
    - No necesitas ser preciso con los nombres: el asistente buscará coincidencias
    - Puedes combinar peticiones: *"Lista los MR abiertos de my-app y dime cuántos issues quedan en el milestone v2"*
    - Si algo falla, reformula la petición con más contexto
