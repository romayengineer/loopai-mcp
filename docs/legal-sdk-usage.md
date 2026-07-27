# Legal: Using the Claude Agent SDK for LoopAI-MCP

## Short answer

**Yes, it is legal.** The Agent SDK is published under the MIT license and explicitly designed for building custom agents on top of Claude Code's capabilities.

## License

The `claude-agent-sdk` Python package is **MIT-licensed** (`License: MIT License (MIT)` on PyPI). The TypeScript package (`@anthropic-ai/claude-agent-sdk`) uses equivalent terms. MIT is among the most permissive open-source licenses — it allows use, modification, distribution, and building commercial products without restriction, subject to the license notice.

Additionally, use of the SDK is governed by [Anthropic's Commercial Terms of Service](https://www.anthropic.com/legal/commercial-terms), including when used to power products and services made available to customers and end users.

## SDK's intended purpose

From the [official documentation](https://code.claude.com/docs/en/agent-sdk/overview):

> *"Build production AI agents with Claude Code as a library."*

> *"Both the TypeScript and Python SDKs bundle a native Claude Code binary for your platform, so you don't need to install Claude Code separately."*

The documentation explicitly compares the SDK to the CLI:

> *"Agent SDK vs Claude Code CLI — Same capabilities, different interface."*

The listed SDK use cases include **"Custom applications"** and **"Production automation."** The SDK is the intended integration path for building your own agent on top of Claude Code — not a hack or loophole.

## What is NOT permitted

There are two specific restrictions:

### Branding

From the [branding guidelines](https://code.claude.com/docs/en/agent-sdk/overview#branding-guidelines):

- **Allowed:** `"Claude Agent"`, `"Claude"`, `"{YourName} Powered by Claude"`
- **Not permitted:** `"Claude Code"`, `"Claude Code Agent"`, or any visual elements that mimic Claude Code
- Your product must maintain its own branding and not appear to be an Anthropic product

### Authentication

> *"Unless previously approved, Anthropic does not allow third party developers to offer claude.ai login or rate limits for their products, including agents built on the Claude Agent SDK."*

LoopAI-MCP avoids both restrictions:
- It uses its own branding (`loopai` / `LoopAI-MCP`), not "Claude Code"
- It authenticates via API keys (the standard method), not `claude.ai` login

## Bottom line

The Agent SDK was published under the MIT license specifically so developers could build tools like LoopAI-MCP. As long as we maintain our own branding and use API key authentication, there is no legal or TOS barrier.
