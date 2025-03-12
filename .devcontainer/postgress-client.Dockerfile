FROM golang:latest

# Install PostgreSQL client for database connectivity
RUN apt-get update && apt-get install -y postgresql-client && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Air for live reloading
RUN go install github.com/air-verse/air@latest

# Set the working directory
WORKDIR /workspace

# Label the image with a descriptive name
LABEL description="Go development environment with PostgreSQL client"
LABEL maintainer="Developer"
LABEL version="1.0"

# Default command is air, but we'll override it in docker-compose
ENTRYPOINT ["air"]