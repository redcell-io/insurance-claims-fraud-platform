# Build context is the repo root (see docker-compose.deploy.yml) — the
# pom's protobuf-maven-plugin reads proto.source.dir =
# ${project.basedir}/../../proto, i.e. the repo-root proto/ tree, not a
# copy inside this service.
FROM eclipse-temurin:25-jdk AS build

ARG MAVEN_VERSION=3.9.9
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" -o /tmp/maven.tar.gz \
    && tar -xzf /tmp/maven.tar.gz -C /opt \
    && rm /tmp/maven.tar.gz
ENV MAVEN_HOME=/opt/apache-maven-${MAVEN_VERSION}
ENV PATH="${MAVEN_HOME}/bin:${PATH}"

# maven-toolchains-plugin (see pom.xml) requires a registered JDK
# 25/temurin toolchain — same requirement as the host's
# ~/.m2/toolchains.xml (see java25-toolchain memory), pointed at this
# image's own JAVA_HOME instead.
RUN mkdir -p /root/.m2 && printf '%s\n' \
    '<?xml version="1.0" encoding="UTF-8"?>' \
    '<toolchains>' \
    '  <toolchain>' \
    '    <type>jdk</type>' \
    '    <provides>' \
    '      <version>25</version>' \
    '      <vendor>temurin</vendor>' \
    '    </provides>' \
    '    <configuration>' \
    "      <jdkHome>${JAVA_HOME}</jdkHome>" \
    '    </configuration>' \
    '  </toolchain>' \
    '</toolchains>' > /root/.m2/toolchains.xml

WORKDIR /src
COPY proto ./proto
COPY services/policy-lookup-svc ./services/policy-lookup-svc
WORKDIR /src/services/policy-lookup-svc
RUN mvn -q -B -DskipTests package

FROM eclipse-temurin:25-jre
COPY --from=build /src/services/policy-lookup-svc/target/*.jar /app/app.jar
EXPOSE 9096 9196
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
