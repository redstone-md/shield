# bash completion for shield (generated via go-flags)
_shield() {
    local args=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    mapfile -t COMPREPLY < <(GO_FLAGS_COMPLETION=1 "${COMP_WORDS[0]}" "${args[@]}" 2>/dev/null)
    return 0
}
complete -o default -F _shield shield
