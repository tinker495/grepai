---
title: AI Agent Skills
description: Install ready-to-use skills to help your AI agent master grepai
---

AI coding agents like Claude Code, Cursor, or Windsurf work better when they understand the tools at their disposal. **grepai-skills** is a collection of 27 skills that teach your AI agent how to use grepai effectively.

## What are Skills?

Skills are knowledge modules that AI agents can load to understand how to use specific tools. Instead of the agent guessing how to use grepai, skills provide:

- Step-by-step instructions for common workflows
- Best practices for writing effective search queries
- Troubleshooting guides for common issues
- Configuration examples for different use cases

## Installation

Install all 27 skills with a single command:

```bash
npx add-skill yoanbernabeu/grepai-skills
```

This works with Claude Code, Cursor, Codex, OpenCode, Windsurf, and 20+ other AI agents.

### Install specific skills

```bash
# Install only search-related skills
npx add-skill yoanbernabeu/grepai-skills --skill grepai-search-basics

# Install globally (available in all projects)
npx add-skill yoanbernabeu/grepai-skills --global

# List all available skills
npx add-skill yoanbernabeu/grepai-skills --list
```

### Manual installation

Copy the `skills/` directory from the repository to:
- **Global**: `~/.claude/skills/` (or `~/.cursor/skills/`, etc.)
- **Project**: `.claude/skills/` (or `.cursor/skills/`, etc.)

## Available Skills

### Getting Started
| Skill | Description |
|-------|-------------|
| `grepai-installation` | Multi-platform installation (Homebrew, shell, Windows) |
| `grepai-ollama-setup` | Install and configure Ollama for local embeddings |
| `grepai-quickstart` | Get searching in 5 minutes |

### Configuration
| Skill | Description |
|-------|-------------|
| `grepai-init` | Initialize grepai in a project |
| `grepai-config-reference` | Complete configuration reference |
| `grepai-ignore-patterns` | Exclude files and directories from indexing |

### Embeddings Providers
| Skill | Description |
|-------|-------------|
| `grepai-embeddings-ollama` | Configure Ollama for local, private embeddings |
| `grepai-embeddings-openai` | Configure OpenAI for cloud embeddings |
| `grepai-embeddings-lmstudio` | Configure LM Studio with GUI interface |

### Storage Backends
| Skill | Description |
|-------|-------------|
| `grepai-storage-gob` | Local file storage (default, simple) |
| `grepai-storage-postgres` | PostgreSQL + pgvector for teams |
| `grepai-storage-qdrant` | Qdrant for high-performance search |

### Semantic Search
| Skill | Description |
|-------|-------------|
| `grepai-search-basics` | Basic semantic code search |
| `grepai-search-advanced` | JSON output, compact mode, AI integration |
| `grepai-search-tips` | Write effective search queries |
| `grepai-search-boosting` | Prioritize source code over tests |

### Call Graph Analysis
| Skill | Description |
|-------|-------------|
| `grepai-trace-callers` | Find all callers of a function |
| `grepai-trace-callees` | Find all functions called by a function |
| `grepai-trace-graph` | Build complete dependency graphs |

### AI Agent Integration
| Skill | Description |
|-------|-------------|
| `grepai-mcp-claude` | Integrate with Claude Code via MCP |
| `grepai-mcp-cursor` | Integrate with Cursor IDE via MCP |
| `grepai-mcp-tools` | Reference for all MCP tools |

### Advanced
| Skill | Description |
|-------|-------------|
| `grepai-workspaces` | Multi-project workspace management |
| `grepai-languages` | Supported programming languages |
| `grepai-troubleshooting` | Diagnose and fix common issues |

## Usage

Once installed, just ask your AI agent naturally:

```
"Help me install and configure grepai"

"Search for authentication code in this project"

"What functions call the Login function?"

"Why are my search results poor?"

"Configure grepai to use OpenAI embeddings"
```

The agent will automatically use the relevant skills to provide accurate guidance.

## External Repository

Skills are maintained in a separate repository to allow independent updates and community contributions.

**Repository**: [github.com/yoanbernabeu/grepai-skills](https://github.com/yoanbernabeu/grepai-skills)

Contributions welcome! See the repository's CONTRIBUTING.md for guidelines.
