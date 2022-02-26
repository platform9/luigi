#!/bin/bash
# First get the tag as the version
TAG=$(git describe --tags HEAD)
if [[ $? -ne 0 ]]
then
    if [[ -z "${TEAMCITY_BUILD_ID}" ]]; then
        TAG=$(git rev-parse --short HEAD)
    else
        TAG=${TEAMCITY_BUILD_ID}
    fi
fi
echo $TAG