# Local Petstore 3 image: official swaggerapi/petstore3 plus an SLF4J 1.7
# binding. The upstream image ships slf4j-api (shaded in jetty-runner and in
# server.war) with no implementation, which prints StaticLoggerBinder errors
# and then uses a no-op logger.
#
# java -jar ignores extra jars, so the runner uses -cp. The webapp gets the
# same jar via extraClasspath, and org.slf4j.impl is a server class so the
# webapp does not share the runner's StaticLoggerBinder (that mismatch is a
# LinkageError). slf4j-simple 1.7.36 matches slf4j-api-1.7.36 (Java 8).
#
# https://github.com/swagger-api/swagger-petstore
# Image is linux/amd64; Apple Silicon runs it via emulation.
# PETSTORE_UPSTREAM is set by run.sh / run.ps1 (--upstream / -Upstream).
ARG PETSTORE_UPSTREAM=swaggerapi/petstore3:latest
FROM ${PETSTORE_UPSTREAM}

RUN wget -q -O /swagger-petstore/slf4j-simple.jar \
      https://repo1.maven.org/maven2/org/slf4j/slf4j-simple/1.7.36/slf4j-simple-1.7.36.jar \
 && echo "2f39bed943d624dfa8f4102d0571283a10870b6aa36f197a8a506f147010c10f  /swagger-petstore/slf4j-simple.jar" \
      | sha256sum -c

COPY jetty-context.xml /swagger-petstore/jetty-context.xml

WORKDIR /swagger-petstore
EXPOSE 8080

CMD ["java", "-DswaggerUrl=openapi.yaml", \
     "-cp", "/swagger-petstore/slf4j-simple.jar:/swagger-petstore/jetty-runner.jar", \
     "org.eclipse.jetty.runner.Runner", \
     "--log", "/var/log/yyyy_mm_dd-requests.log", \
     "/swagger-petstore/jetty-context.xml"]
