FROM alpine

RUN apk update --no-cache && apk add --no-cache ca-certificates tzdata

# Set timezone if necessary
#ENV TZ UTC
ENV USER=gouser
ENV UID=10001

RUN adduser \
    --disabled-password \
    --gecos "" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

ADD {{.ProjectFolderName}} /app/{{.ProjectFolderName}}
WORKDIR /app
USER gouser:gouser

ENTRYPOINT ["./{{.ProjectFolderName}}"]