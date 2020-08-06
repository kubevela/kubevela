package cmd

import (
	"bytes"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
)

func NewCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "completion [bash|zsh]",
		DisableFlagsInUseLine: true,
		Short:                 "Output shell completion code for the specified shell (bash or zsh)",
		Long:                  completionLong(),
		ValidArgs:             []string{"bash", "zsh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(cmd, args)
		},
		Example: completionExample(),
	}
}

func runCompletion(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("Shell not specified.")
	}
	if len(args) > 1 {
		return errors.New("Too many arguments. Expected only the shell type.")
	}
	complet := args[0]
	if complet == "bash" {
		err := cmd.Root().GenBashCompletion(os.Stdout)
		if err != nil {
			return err
		}
	} else if complet == "zsh" {
		err := runCompletionZsh(os.Stdout, cmd.Root())
		if err != nil {
			return err
		}
	} else {
		return errors.New("Parameter error! Please input bash or zsh")
	}
	return nil
}

func completionLong() string {
	return `
Output shell completion code for the specified shell (bash or zsh). The shell code must be evaluated to provide
interactive completion of rudrx commands.  This can be done by sourcing it from the .bash_profile.
`
}

func completionExample() string {
	return `
# bash
rudrx completion bash > ~/.kube/rudrx.bash.inc
printf "
# rudrx shell completion
source '$HOME/.kube/rudrx.bash.inc'
" >> $HOME/.bash_profile
source $HOME/.bash_profile

# add to $HOME/.zshrc
source <(rudrx completion zsh)
# or
rudrx completion zsh > "${fpath[1]}/_rudr"
`
}

func runCompletionZsh(out io.Writer, cmd *cobra.Command) error {
	zshHead := "#compdef rudrx\n"

	out.Write([]byte(zshHead))

	zshInitialization := `
__rudrx_bash_source() {
	alias shopt=':'
	emulate -L sh
	setopt kshglob noshglob braceexpand

	source "$@"
}

__rudrx_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift

		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__rudrx_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}

__rudrx_compgen() {
	local completions w
	completions=( $(compgen "$@") ) || return $?

	# filter by given word as prefix
	while [[ "$1" = -* && "$1" != -- ]]; do
		shift
		shift
	done
	if [[ "$1" == -- ]]; then
		shift
	fi
	for w in "${completions[@]}"; do
		if [[ "${w}" = "$1"* ]]; then
			echo "${w}"
		fi
	done
}

__rudrx_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}

__rudrx_ltrim_colon_completions()
{
	if [[ "$1" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
		# Remove colon-word prefix from COMPREPLY items
		local colon_word=${1%${1##*:}}
		local i=${#COMPREPLY[*]}
		while [[ $((--i)) -ge 0 ]]; do
			COMPREPLY[$i]=${COMPREPLY[$i]#"$colon_word"}
		done
	fi
}

__rudrx_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}

__rudrx_filedir() {
	# Don't need to do anything here.
	# Otherwise we will get trailing space without "compopt -o nospace"
	true
}

autoload -U +X bashcompinit && bashcompinit

# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q 'GNU\|BusyBox'; then
	LWORD='\<'
	RWORD='\>'
fi

__rudrx_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/_get_comp_words_by_ref "\$@"/_get_comp_words_by_ref "\$*"/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__rudrx_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__rudrx_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__rudrx_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__rudrx_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__rudrx_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/builtin declare/g" \
	-e "s/\\\$(type${RWORD}/\$(__rudrx_type/g" \
	<<'BASH_COMPLETION_EOF'
`
	out.Write([]byte(zshInitialization))

	buf := new(bytes.Buffer)
	cmd.GenBashCompletion(buf)
	out.Write(buf.Bytes())

	zshTail := `
BASH_COMPLETION_EOF
}

__rudrx_bash_source <(__rudrx_convert_bash_to_zsh)
`
	out.Write([]byte(zshTail))
	return nil
}
