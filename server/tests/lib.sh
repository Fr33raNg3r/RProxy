#!/bin/bash
# 极简测试框架：断言失败只记录，由 finish 统一汇总并以非零状态退出
T_PASS=0
T_FAIL=0

ok()   { T_PASS=$((T_PASS + 1)); echo "  通过：$1"; }
fail() { T_FAIL=$((T_FAIL + 1)); echo "  失败：$1"; }

# assert_eq <实际> <期望> <说明>
assert_eq() {
    if [[ "$1" == "$2" ]]; then ok "$3"; else fail "$3（期望 [$2]，实际 [$1]）"; fi
}

# assert_file <路径> <说明> / assert_no_file <路径> <说明>
assert_file()    { if [[ -f "$1" ]]; then ok "$2"; else fail "$2（缺少 $1）"; fi; }
assert_no_file() { if [[ ! -e "$1" ]]; then ok "$2"; else fail "$2（不应存在 $1）"; fi; }

# assert_grep <正则> <文件> <说明> / assert_no_grep：用 grep -E 匹配文件内容
assert_grep()    { if grep -qE -- "$1" "$2" 2>/dev/null; then ok "$3"; else fail "$3（$2 中没有匹配 $1）"; fi; }
assert_no_grep() { if grep -qE -- "$1" "$2" 2>/dev/null; then fail "$3（$2 中不应匹配 $1）"; else ok "$3"; fi; }

# assert_match <正则> <字符串> <说明> / assert_no_match
assert_match()    { if grep -qE -- "$1" <<<"$2"; then ok "$3"; else fail "$3（没有匹配 $1）"; fi; }
assert_no_match() { if grep -qE -- "$1" <<<"$2"; then fail "$3（不应匹配 $1）"; else ok "$3"; fi; }

finish() {
    echo
    echo "通过 ${T_PASS}，失败 ${T_FAIL}"
    [[ $T_FAIL -eq 0 ]]
}
