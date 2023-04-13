package foo

import "tool/exec"

city: "Amsterdam"
who: *"World" | string @tag(who)

// Say hello!
command: hello: {
  print: exec.Run & {
    cmd: "echo Hello \(who)! Welcome to \(city)."
  }
}
