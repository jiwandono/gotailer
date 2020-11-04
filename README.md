# gotailer

This small application tails and sends the output of your command to web interface via websocket.

Usage:

`./gotailer <ADDRESS:PORT> <WORKING DIR> <COMMAND> [ARGUMENTS]`

Features:

- Like the usual output tailer, but on web
- ANSI color support via [ansi_up](https://github.com/drudru/ansi_up)
