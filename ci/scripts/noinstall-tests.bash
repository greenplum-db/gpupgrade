#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

apt update
apt install -y rsync

./bats/install.sh /usr/local

# Run unit tests as a non-root user since some tests rely on specifying file
# permissions which are overridden by root.
adduser  --disabled-password --gecos "" --ingroup tty --shell /bin/bash gpadmin
chmod -R a+w gpupgrade_src

su gpadmin -c '
  set -ex

  export TERM=linux
  export GOFLAGS="-mod=readonly" # do not update dependencies during build

  cd gpupgrade_src
  make
  make check --keep-going
'
