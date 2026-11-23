# Build context is the repo root (see docker-compose.deploy.yml) — the
# pom's protobuf-maven-plugin reads proto.source.dir =
# ${project.basedir}/../../proto, i.e. the repo-root proto/ tree, not a
# copy inside this service.
FROM eclipse-temurin:25-jdk AS build

ARG MAVEN_VERSION=3.9.9
# bsdtar (libarchive-tools), not GNU tar: GNU tar hits "Function not
# implemented" extracting this archive under QEMU's user-mode emulation
# (buildx --platform linux/arm64 cross-build on an amd64 host, e.g.
# scripts/build-and-push-oci.sh) — a QEMU syscall-coverage gap, confirmed
# reproducible even after refreshing binfmt handlers and recreating the
# builder. Doesn't affect a native build (e.g. directly on arm64
# hardware); bsdtar sidesteps the gap either way.
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl libarchive-tools \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" -o /tmp/maven.tar.gz \
    && bsdtar -xzf /tmp/maven.tar.gz -C /opt \
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
COPY services/address-normalization-svc ./services/address-normalization-svc
WORKDIR /src/services/address-normalization-svc
RUN mvn -q -B -DskipTests package

FROM eclipse-temurin:25-jre
COPY --from=build /src/services/address-normalization-svc/target/*.jar /app/app.jar
EXPOSE 9092 9192
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
