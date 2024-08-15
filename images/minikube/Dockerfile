FROM debian:bookworm

# Since we are downloading binaries, we need the arch.
ARG ARCH=MUST_SET

# Base to install other components.
USER root
RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install -y ca-certificates curl

# Install the docker cli.
# https://docs.docker.com/engine/install/debian/
ARG DOCKER_CLIENT_VERSION=MUST_SET
RUN curl -Lo /tmp/docker-cli.deb https://download.docker.com/linux/debian/dists/bookworm/pool/stable/${ARCH}/docker-ce-cli_${DOCKER_CLIENT_VERSION}-1~debian.12~bookworm_${ARCH}.deb
RUN dpkg -i /tmp/docker-cli.deb

# Install minikube.
# https://minikube.sigs.k8s.io/docs/start/
ARG MINIKUBE_VERSION=MUST_SET
RUN curl -Lo /usr/local/bin/minikube https://storage.googleapis.com/minikube/releases/${MINIKUBE_VERSION}/minikube-linux-${ARCH}
RUN chmod +x /usr/local/bin/minikube

# Set some setting for minikube, such as the home folder.
ENV MINIKUBE_IN_STYLE=0
ENV MINIKUBE_HOME=/app/.minikube
ENTRYPOINT ["minikube"]
