#!/bin/bash
set -e

OPS_HOME="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$(dirname "${OPS_HOME}")"/.. || exit 1

echo "Bumping version"
mmb=($(cat VERSION | tr '.' '\n'))

MAJOR="${mmb[0]}"
MINOR="${mmb[1]}"
BUILD="${mmb[2]}"

if [[ "$2" == "-major" ]]
then
  MAJOR="$(($MAJOR + 1))"
elif [[ "$2" == "-minor" ]]
then
  MINOR="$(($MINOR + 1))"
else
  BUILD="$((BUILD + 1))"
fi

VERSION="${MAJOR}.${MINOR}.${BUILD}"
echo $VERSION > VERSION

git checkout -b devops/version-bump-${VERSION}

git add .
git commit -m "NOTKT: Bumped version to ${VERSION}"
git push -u origin devops/version-bump-${VERSION}
