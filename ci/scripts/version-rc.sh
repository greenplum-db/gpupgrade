#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

# todo: is there a cleaner way to get this format?
version=$(git -C ./gpupgrade_src describe | perl -pe 's/(.*)-([0-9]*)-(g[0-9a-f]*)/\1+dev.\2.\3/')

cp bin_gpupgrade/gpupgrade gpupgrade_bin_path/gpupgrade-rc-$version-linux_x86_64
