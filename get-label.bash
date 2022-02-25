#!/bin/bash
# First get the tag as the version
TAG=$(git describe --tags HEAD)
if [[ $? -ne 0 ]]
then
    # if we cannot get the tag, lets use the <branch>-<abbrev-sha1> as the tag name
    TAG=$(git rev-parse --abbrev-ref HEAD)-$(git rev-parse --short HEAD)
fi
echo $TAG