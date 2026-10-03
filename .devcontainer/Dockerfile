# Dependabot (devcontainers/docker ecosystems) keeps the base image current.
FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

ARG USERNAME=vscode
# Keep in sync with .golangci.yml and .github/workflows/lint.yml
ARG GOLANGCI_LINT_VERSION=v2.14.0
# Editor tooling only. Project tools (mockery, govulncheck, go-licenses) are `tool` directives in go.mod: `go tool <name>`.
ARG GOPLS_VERSION=v0.23.0
ARG DELVE_VERSION=v1.27.2
ARG GOMODIFYTAGS_VERSION=v1.17.0
ARG IMPL_VERSION=v1.5.0
ARG GOTESTS_VERSION=v1.9.0
ARG GORELEASER_VERSION=v2.18.2

ENV LANG=en_US.UTF-8
ENV DEBIAN_FRONTEND=noninteractive
ENV GOPATH=/home/${USERNAME}/go
ENV GOBIN=/home/${USERNAME}/go/bin
# Prepend GOBIN so the pinned tools win over the base image's copies in /go/bin.
ENV PATH=${GOBIN}:${PATH}

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash-completion \
    ca-certificates \
    curl \
    git \
    jq \
    make \
    netcat-traditional \
    vim \
    wget && \
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/*

USER ${USERNAME}

RUN mkdir -p "${GOBIN}" && \
    curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/${GOLANGCI_LINT_VERSION}/install.sh" | sh -s -- -b "${GOBIN}" "${GOLANGCI_LINT_VERSION}" && \
    go install "golang.org/x/tools/gopls@${GOPLS_VERSION}" && \
    go install "github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}" && \
    go install "github.com/fatih/gomodifytags@${GOMODIFYTAGS_VERSION}" && \
    go install "github.com/josharian/impl@${IMPL_VERSION}" && \
    go install "github.com/cweill/gotests/gotests@${GOTESTS_VERSION}" && \
    go install "github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}" && \
    go clean -cache -modcache && \
    echo 'source /usr/share/bash-completion/bash_completion' >> "/home/${USERNAME}/.bashrc"

USER root

# Persist bash history.
RUN SNIPPET="export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" && \
    mkdir /commandhistory && \
    touch /commandhistory/.bash_history && \
    chown -R "${USERNAME}" /commandhistory && \
    echo "$SNIPPET" >> "/home/${USERNAME}/.bashrc"
