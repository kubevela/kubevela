### GitHub & kubevela automation

The bot is configured via [issue-commands.json](https://github.com/kubevela/kubevela/blob/master/.github/issue-commands.json) 
and some other GitHub [workflows](https://github.com/kubevela/kubevela/blob/master/.github/workflows).
By default, users with write access to the repo is allowed to use the comments, 
the [userlist](https://github.com/kubevela/kubevela/blob/master/.github/comment.userlist) 
file is for adding additional members who do not have access and want to contribute to the issue triage.

Comment commands:
* Write the word `/needsInvestigation` in a comment, and the bot will add the corresponding label.
* Write the word `/needsMoreInfo` in a comment, and the bot will add the correct label and standard message.
* Write the word `/duplicate #<number>` to have `type/duplicate` label, the issue number is required for remind where is the other issue.
* Write the word `/type/*` in a comment, and the bot will add the corresponding label `/type/*`.
* Write the word `/area/*` in a comment, and the bot will add the corresponding label `/area/*`.
* Write the word `/priority/*` in a comment, and the bot will add the corresponding label `/priority/*`.

The `*` mention above represent a specific word. Please read the details about label category in [ISSUE_TRIAGE.md](https://github.com/kubevela/kubevela/blob/master/ISSUE_TRIAGE.md)  

Label commands:

* Add label `bot/question` the bot will close with standard question message and add label `type/question`
* Add label `bot/needs more info` for bot to request more info (or use comment command mentioned above)
* Add label `bot/no new info` for bot to close an issue where we asked for more info but has not received any updates in at least 14 days.
* Add label `bot/duplicate` to have `type/duplicate` label & the bot will close issue with an appropriate message.
* Add label `bot/close feature request` for bot to close a feature request with standard message.

Assign:
When you participating in an issue area, and you want to assign to others
to distribute this task or self-assign to give a solution. You can use the comment bellow.
* Write the word `/assign githubname` in a comment, the robot will automatically assign to the corresponding person.
* Specially, write the word `/assign` in a comment, you can assgin this task to yourself.  