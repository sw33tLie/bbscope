# Burp Suite Integration

bbscope includes a Burp Suite script that checks whether HTTP request targets are in scope by querying the bbscope REST API.

## Setup

1. Start the bbscope web server (or use a hosted instance).
2. In Burp Suite, load the script from `website/find-scope-burp-action.java`.
3. Configure the script to point to your bbscope instance URL.

## How it works

The script intercepts HTTP requests in Burp and checks each hostname against the bbscope API's wildcard/domain list. If the target matches an in-scope entry, it's flagged accordingly.

This is useful for passive scope validation during testing — you can see at a glance whether a target you're interacting with is actually in scope for a bug bounty program.

## Requirements

- A running bbscope web server with the REST API accessible
- Programs polled and stored in the database
