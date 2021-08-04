#!/usr/bin/env bash
set -e

OPS_HOME="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$(dirname "${OPS_HOME}")"/.. || exit 1
echo $PWD

# this is the tag point
SHA=$(git rev-parse --verify HEAD)

VERSION=$(cat VERSION)

TAG_NAME=""
TYPE="DEV"
DATE=$(date -u +"%Y%m%d%H%M%SZ")
if [ "$2" == "-release" ]; then
    TYPE="RELEASE"
    TAG_NAME="release/${VERSION}"
else
    TAG_NAME="dev/${VERSION}-${DATE}"
fi

echo "Tagging ${VERSION} / ${SHA} / ${TAG_NAME}"

git tag -a ${TAG_NAME} -m "[${TYPE}] ${DATE}"
git push --tags

${OPS_HOME}/bump.sh

echo "DONE"