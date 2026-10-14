package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
)

const BashCompletionPrelude = `
if [[ $(type -t _get_comp_words_by_ref 2>/dev/null) != function ]]; then
_get_comp_words_by_ref()
{
    local current_index current_word previous_word

    while [[ $# -gt 0 && $1 == -* ]]; do
        case "$1" in
            -n)
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done

    current_index=${COMP_CWORD:-0}
    current_word="${COMP_WORDS[current_index]}"
    previous_word=""
    if (( current_index > 0 )); then
        previous_word="${COMP_WORDS[current_index-1]}"
    fi

    while [[ $# -gt 0 ]]; do
        case "$1" in
            cur)
                printf -v "$1" '%s' "$current_word"
                ;;
            prev)
                printf -v "$1" '%s' "$previous_word"
                ;;
            words)
                eval "$1=(\"\${COMP_WORDS[@]}\")"
                ;;
            cword)
                printf -v "$1" '%s' "$current_index"
                ;;
        esac
        shift
    done
}
fi

`

// RenderCompletion is shared by shell emission and generated hosted resources.
func RenderCompletion(root *cobra.Command, shell string, out io.Writer, noDescriptions bool) error {
	switch shell {
	case "bash":
		if _, err := io.WriteString(out, BashCompletionPrelude); err != nil {
			return err
		}
		return root.GenBashCompletionV2(out, !noDescriptions)
	case "zsh":
		if noDescriptions {
			return root.GenZshCompletionNoDesc(out)
		}
		return root.GenZshCompletion(out)
	case "fish":
		return root.GenFishCompletion(out, !noDescriptions)
	case "powershell":
		if noDescriptions {
			return root.GenPowerShellCompletion(out)
		}
		return root.GenPowerShellCompletionWithDesc(out)
	default:
		return fmt.Errorf("unsupported completion shell %q", shell)
	}
}
