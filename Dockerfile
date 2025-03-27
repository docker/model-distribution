FROM golang AS builder

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./

# Download dependencies (this layer will be cached if dependencies don't change)
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN make build

FROM ignaciolopezluna020/llama.cpp:cpu AS push

ARG HF_MODEL
ARG REPOSITORY
ARG WEIGHTS
ARG LICENCES_PATH
ARG QUANTIZATION
ARG SKIP_F16="false"

# Install git-lfs to handle Hugging Face repositories
RUN apt-get update && apt-get install -y git-lfs && \
    git lfs install

# Copy the modified entrypoint script
COPY scripts/model-push/entrypoint.sh /entrypoint.sh
COPY scripts/model-push/push-model.sh /push-model.sh
RUN chmod +x /entrypoint.sh
RUN chmod +x /push-model.sh
COPY --from=builder /go/bin/model-distribution-tool /usr/local/bin/model-distribution-tool

RUN --mount=type=secret,id=hf-token,env=HUGGINGFACE_TOKEN \
    --mount=type=secret,id=docker-user,env=DOCKER_USERNAME \
    --mount=type=secret,id=docker-password,env=DOCKER_PASSWORD \
    /push-model.sh \
    --hf-model $HF_MODEL \
    --hf-token $HUGGINGFACE_TOKEN \
    --repository $REPOSITORY \
    --weights $WEIGHTS \
    --models-dir /app/models \
    --licenses $LICENCES_PATH \
    --quantization $QUANTIZATION

# Allow passing Hugging Face API Token as an environment variable
#ENV HUGGINGFACE_TOKEN=""

#ENTRYPOINT ["/entrypoint.sh"]
