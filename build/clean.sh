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
          rm -rf $1/docs
          set +ex
        fi  
      fi
  done
}

# build
buildProto $service

buildSwagger $service


