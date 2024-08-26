#!/bin/bash
mkdir -p _build
cd _build
mkdir -p docker_out
rm -rf sources
git clone $(git remote get-url origin) sources
cd sources
git fetch --tags
ver=$(git describe --tags `git rev-list --tags --max-count=1`)
git checkout $ver

CGO_ENABLED=0 go build -ldflags="-s -w" -o ../{{.ProjectFolderName}} .

# A second run is needed to build the final image.
cd ..
docker build -f sources/Dockerfile --no-cache -t {{.GoModuleName}}:${ver} .
docker push {{.GoModuleName}}:${ver}
rm -rf sources {{.ProjectFolderName}}