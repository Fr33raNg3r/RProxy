#!/bin/bash
# 依次运行 tests/test_*.sh，任何一个失败就以非零状态退出
cd "$(dirname "$0")" || exit 1
rc=0
for t in test_*.sh; do
    echo "=== ${t}"
    bash "$t" || rc=1
done
exit $rc
