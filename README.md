# Arisu

Seamless AI coding and editing on the terminal

Prompt and idea borrowed from Victor Taelin's AI-scripts.

## Usage

### Setting different models
```sh
arisu --setmodel gemini/grok
```

### Activate autorun and autoedit

By default, Arisu asks for your confirmation before editing files and running commands.
If you want to modify this behavior, use the following flags

```sh
arisu --auto-edit true/false
arisu --auto-run true/false
```