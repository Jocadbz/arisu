# Arisu

Seamless AI coding and editing on the terminal

Prompt and idea borrowed from Victor Taelin's AI-scripts.

## Usage

### Setting different models
`arisu --setmodel <model>`

**Supported Models:**
- **Gemini**: `gemini` (defaults to `gemini-2.0-flash`), `gemini-2.0-flash`, `gemini-2.5-flash`, `gemini-2.5-pro`
- **Grok**: All models except image generation
- **OpenAI**: `gpt-4.1-mini`, `gpt-4.1`, `gpt-4o`, `gpt-4o-mini`, `o3`, `gpt-3.5-turbo`

### Activate autorun and autoedit

By default, Arisu asks for your confirmation before editing files and running commands.
If you want to modify this behavior, use the following flags

`arisu --auto-edit true/false`
`arisu --auto-run true/false`
