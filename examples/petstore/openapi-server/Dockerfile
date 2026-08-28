# Local Petstore 3 image: official swaggerapi/petstore3 plus an optional SLF4J
# 1.7 binding. The upstream image ships slf4j-api (shaded in jetty-runner and
# in server.war) with no implementation, which prints StaticLoggerBinder errors
# and then uses a no-op logger.
#
# java -jar ignores extra jars, so the runner uses -cp. The webapp gets the
# same jar via extraClasspath, and org.slf4j.impl is a server class so the
# webapp does not share the runner's StaticLoggerBinder (that mismatch is a
# LinkageError). The default binding is slf4j-simple 1.7.36 (matches
# slf4j-api-1.7.36, Java 8). --slf4j may supply any 1.7 binding (simple,
# log4j12, jdk14, …); it is installed as slf4j-impl.jar.
#
# https://github.com/swagger-api/swagger-petstore
# Image is linux/amd64; Apple Silicon runs it via emulation.
# Build args are set by run.sh / run.ps1 (--upstream, --slf4j, --no-slf4j).
ARG PETSTORE_UPSTREAM=swaggerapi/petstore3:latest
FROM ${PETSTORE_UPSTREAM}

ARG SLF4J_MODE=url
ARG SLF4J_URL=https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar
ARG SLF4J_SHA256=2f39bed943d624dfa8f4102d0571283a10870b6aa36f197a8a506f147010c10f

COPY jetty-context.xml /swagger-petstore/jetty-context.xml
COPY jetty-context-noslf4j.xml /tmp/jetty-context-noslf4j.xml
COPY slf4j.bundle/ /tmp/slf4j.bundle/

RUN set -e; \
    case "$SLF4J_MODE" in \
      skip) \
        cp /tmp/jetty-context-noslf4j.xml /swagger-petstore/jetty-context.xml ;; \
      file) \
        cp /tmp/slf4j.bundle/slf4j-impl.jar /swagger-petstore/slf4j-impl.jar ;; \
      url) \
        wget -q -O /swagger-petstore/slf4j-impl.jar "$SLF4J_URL"; \
        if [ -n "$SLF4J_SHA256" ]; then \
          echo "$SLF4J_SHA256  /swagger-petstore/slf4j-impl.jar" | sha256sum -c; \
        fi ;; \
      *) \
        echo "unknown SLF4J_MODE=$SLF4J_MODE" >&2; exit 1 ;; \
    esac

WORKDIR /swagger-petstore
EXPOSE 8080

CMD ["java", "-DswaggerUrl=openapi.yaml", \
     "-cp", "/swagger-petstore/slf4j-impl.jar:/swagger-petstore/jetty-runner.jar", \
     "org.eclipse.jetty.runner.Runner", \
     "--log", "/var/log/yyyy_mm_dd-requests.log", \
     "/swagger-petstore/jetty-context.xml"]
