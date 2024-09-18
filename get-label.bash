#!/bin/bash
#First set the suffix to some value, either Teamcity build-id or <abbrev-sha1>
if [[ -z "${{ github.run_number }}" ]]; then
    SUFFIX=$(git rev-parse --short HEAD)
else
    SUFFIX=${{ github.run_number }}
fi

# Get the tag as the version
TAG=$(git describe --tags HEAD)
if [[ $? -ne 0 ]]
then
    # if we cannot get the tag, lets use the <branch>-pmk-<suffix> as the tag name
    echo "TAG=$(git rev-parse --abbrev-ref HEAD | sed 's/[^a-zA-Z0-9_.]/-/g')-pmk-${SUFFIX}" >> $GITHUB_ENV
else
    echo "TAG=$(echo $TAG | sed 's/-.*//')-pmk-${SUFFIX}" >> $GITHUB_ENV
fi
echo ${{ env.TAG }}
