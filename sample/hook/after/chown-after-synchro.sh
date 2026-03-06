#!/bin/bash
set -eu

: "${dst_files_root:?dst_files_root is required}"

# Chown the synced destination tree. Adjust user:group before use.
chown -R user:group "${dst_files_root}"
