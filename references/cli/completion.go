/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
)

const completionDesc = `Output shell completion code for the specified shell (bash or zsh). 
The shell code must be evaluated to provide interactive completion of vela commands.`

const bashCompDesc = `Generate the autocompletion script for Vela for the bash shell.

To load completions in your current shell session:
$ source <(vela completion bash)

To load completions for every new session, execute once:
Linux:
  $ vela completion bash > /etc/bash_completion.d/vela
MacOS:
  $ vela completion bash > /usr/local/etc/bash_completion.d/vela
`

const zshCompDesc = `Generate the autocompletion script for Vela for the zsh shell.

To load completions in your current shell session:
$ source <(vela completion zsh)

To load completions for every new session, execute once:
$ vela completion zsh > "${fpath[1]}/_vela"
`

// NewCompletionCommand Output shell completion code for the specified shell (bash or zsh)
func NewCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Output shell completion code for the specified shell (bash or zsh)",
		Long:  completionDesc,
		Args:  nil,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}

	bash := &cobra.Command{
		Use:                   "bash",
		Short:                 "generate autocompletions script for bash",
		Long:                  bashCompDesc,
		Args:                  nil,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletionBash(os.Stdout, cmd)
		},
	}

	zsh := &cobra.Command{
		Use:                   "zsh",
		Short:                 "generate autocompletions script for zsh",
		Long:                  zshCompDesc,
		Args:                  nil,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletionZsh(os.Stdout, cmd)
		},
	}

	cmd.AddCommand(bash, zsh)

	return cmd
}

func runCompletionBash(out io.Writer, cmd *cobra.Command) error {
	err := cmd.Root().GenBashCompletion(out)

	if binary := filepath.Base(os.Args[0]); binary != "vela" {
		renamedBinaryHook := `
# Hook the command used to generate the completion script
# to the vela completion function to handle the case where
# the user renamed the vela binary
if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __start_vela %[1]s
else
    complete -o default -o nospace -F __start_vela %[1]s
fi
`
		_, err = fmt.Fprintf(out, renamedBinaryHook, binary)
		if err != nil {
			return err
		}
	}
	return err
}

func runCompletionZsh(out io.Writer, cmd *cobra.Command) error {
	zshInitialization := `#compdef vela

__vela_bash_source() {
	alias shopt=':'
	alias _expand=_bash_expand
	alias _complete=_bash_comp
	emulate -L sh
	setopt kshglob noshglob braceexpand
	source "$@"
}
__vela_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift
		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__vela_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}
__vela_compgen() {
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
			# Use printf instead of echo beause it is possible that
			# the value to print is -n, which would be interpreted
			# as a flag to echo
			printf "%s\n" "${w}"
		fi
	done
}
__vela_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}
__vela_ltrim_colon_completions()
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
__vela_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}
__vela_filedir() {
	local RET OLD_IFS w qw
	__debug "_filedir $@ cur=$cur"
	if [[ "$1" = \~* ]]; then
		# somehow does not work. Maybe, zsh does not call this at all
		eval echo "$1"
		return 0
	fi
	OLD_IFS="$IFS"
	IFS=$'\n'
	if [ "$1" = "-d" ]; then
		shift
		RET=( $(compgen -d) )
	else
		RET=( $(compgen -f) )
	fi
	IFS="$OLD_IFS"
	IFS="," __debug "RET=${RET[@]} len=${#RET[@]}"
	for w in ${RET[@]}; do
		if [[ ! "${w}" = "${cur}"* ]]; then
			continue
		fi
		if eval "[[ \"\${w}\" = *.$1 || -d \"\${w}\" ]]"; then
			qw="$(__vela_quote "${w}")"
			if [ -d "${w}" ]; then
				COMPREPLY+=("${qw}/")
			else
				COMPREPLY+=("${qw}")
			fi
		fi
	done
}
__vela_quote() {
	if [[ $1 == \'* || $1 == \"* ]]; then
		# Leave out first character
		printf %q "${1:1}"
	else
		printf %q "$1"
	fi
}
autoload -U +X bashcompinit && bashcompinit
# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q 'GNU\|BusyBox'; then
	LWORD='\<'
	RWORD='\>'
fi
__vela_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/_get_comp_words_by_ref "\$@"/_get_comp_words_by_ref "\$*"/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__vela_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__vela_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__vela_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__vela_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__vela_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/builtin declare/g" \
	-e "s/\\\$(type${RWORD}/\$(__vela_type/g" \
	-e 's/aliashash\["\(.\{1,\}\)"\]/aliashash[\1]/g' \
	-e 's/FUNCNAME/funcstack/g' \
	<<'BASH_COMPLETION_EOF'
`
	_, err := out.Write([]byte(zshInitialization))
	if err != nil {
		return err
	}

	if err = runCompletionBash(out, cmd); err != nil {
		return err
	}

	zshTail := `
BASH_COMPLETION_EOF
}
__vela_bash_source <(__vela_convert_bash_to_zsh)
`
	_, err = out.Write([]byte(zshTail))
	return err
}
