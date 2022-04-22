#!/bin/bash

pwd=`pwd`
ls=`ls`
service=$pwd"/app/game"
length=${#pwd}

buildProto(){
  for file in `ls $1`
    do
      if [ -d $1"/"$file ]
      then
          if [[ $file != '.' && $file != '..' ]]
          then
              buildProto $1"/"$file
          fi
      else
        if [ "${file##*.}"x = "proto"x ]
        then
          path=${1: $length+1}
          name=${file%.*}
          gofile="./"$path/$name"*.go"
          jsonfile="./"$path/$name"*.json"
          protofile="./"$path/$name".proto"
          set -ex
          rm -rf $gofile $jsonfile
          protoc --proto_path=./ --gofast_out=plugins=grpc:./ --gofast_opt=paths=source_relative $protofile
          protoc --proto_path=./ --gin_out=./ --gin_opt=paths=source_relative $protofile
          set +ex
        fi     
      fi
  done
}

buildSwagger(){
  for file in `ls $1`
    do
      if [ -d $1"/"$file ]
      then
        if [[ $file != '.' && $file != '..' ]]
        then
          set -ex
          mkdir $1"/docs"
          protoc --proto_path=./ --swagger_out=allow_delete_body=true,allow_merge=true,logtostderr=true:./app/game/docs ./app/game/*.proto
          createSwagger $1
          set +ex
        fi  
      fi
  done
}

createSwagger(){
  cat > $1/docs/swagger.go << EOF
package docs

import (
	_ "embed"
)

var (
	//go:embed *swagger.json
	swagger string
)

//get docs
func GetDocs() string {
	return swagger
}
EOF
}

# build
buildProto $service

buildSwagger $service


