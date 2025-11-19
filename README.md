# Arisu

Seamless AI coding and editing on the terminal

Prompt and idea borrowed from Victor Taelin's AI-scripts.

## Features

- Interactive terminal-based AI assistant for coding and file editing
- Support for multiple AI providers: Gemini, Grok, OpenAI, and OpenRouter
- Automatic file editing and command execution with confirmation prompts
- Agentic mode for step-by-step automation
- Conversation logging and history management
- Multi-line input support

## Installation

1. Install Go 1.24.0 or later
2. Clone the repository and build:
   ```
   git clone <repository-url>
   cd arisu
   go build -o arisu .
   ```
3. Make it executable and move to your PATH:
   ```
   chmod +x arisu
   sudo mv arisu /usr/local/bin/
   ```

## Configuration

Arisu stores configuration in `~/.config/arisu/config.json`. API keys are stored securely and only required once per provider.

## Usage

### Basic Usage

Launch Arisu with an optional initial prompt:
```
# Start interactive session
arisu

# Start with initial prompt
arisu "Help me refactor my Go code"
```

Press Enter to send a prompt. Use Ctrl+Enter to insert a new line without sending. Type `exit` to quit.

### Setting Models and Configuration

```
# Set default model
arisu --setmodel <model>

# Configure auto-edit (true/false)
arisu --auto-edit true

# Configure auto-run commands (true/false)
arisu --auto-run false
```

### Supported Models

**Gemini (Google):**
- `gemini` (defaults to `gemini-2.0-flash`)
- `gemini-2.0-flash`
- `gemini-2.5-flash`
- `gemini-2.5-pro`
- `gemini-3-pro-preview`

**Grok (xAI):**
- Any model (text-only, no image generation)

**OpenAI:**
- `gpt-4.1-mini`
- `gpt-4.1`
- `gpt-4o`
- `gpt-4o-mini`
- `o3`
- `gpt-3.5-turbo`

**OpenRouter:**
- Use format `openrouter-<model>` for any model available on OpenRouter
- Examples: `openrouter-openrouter/sonoma-dusk-alpha`, `openrouter-deepcogito/cogito-v2-preview-llama-109b-moe`



