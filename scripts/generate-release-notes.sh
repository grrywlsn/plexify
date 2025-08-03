#!/bin/bash

# Generate release notes manually for testing
# Usage: ./scripts/generate-release-notes.sh [version] [previous-tag]

set -e

VERSION=${1:-"v1.0.0"}
PREVIOUS_TAG=${2:-""}

echo "Generating release notes for version: $VERSION"
echo "Previous tag: ${PREVIOUS_TAG:-"none (first release)"}"
echo ""

# Get merged PRs since the last release
if [ -n "$PREVIOUS_TAG" ]; then
    COMMIT_RANGE="$PREVIOUS_TAG..HEAD"
    MERGED_PRS=$(git log --merges --pretty=format:"%H %s" $COMMIT_RANGE | grep -E "Merge pull request #[0-9]+" || echo "")
else
    MERGED_PRS=$(git log --merges --pretty=format:"%H %s" | grep -E "Merge pull request #[0-9]+" || echo "")
fi

# Generate release notes
RELEASE_NOTES="## Plexify $VERSION"

if [ -n "$MERGED_PRS" ]; then
    RELEASE_NOTES="$RELEASE_NOTES

### Changes in this release

This release includes the following merged pull requests:"
    
    # Process each merged PR
    while IFS= read -r line; do
        if [[ $line =~ Merge\ pull\ request\ #([0-9]+) ]]; then
            PR_NUMBER="${BASH_REMATCH[1]}"
            COMMIT_HASH=$(echo "$line" | awk '{print $1}')
            
            echo "Processing PR #$PR_NUMBER..."
            
            # Get PR details using GitHub API (if GITHUB_TOKEN is set)
            if [ -n "$GITHUB_TOKEN" ]; then
                PR_INFO=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
                    "https://api.github.com/repos/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/pulls/$PR_NUMBER")
                
                PR_TITLE=$(echo "$PR_INFO" | jq -r '.title // "Unknown"')
                PR_USER=$(echo "$PR_INFO" | jq -r '.user.login // "Unknown"')
                PR_LABELS=$(echo "$PR_INFO" | jq -r '.labels[].name // empty' | tr '\n' ', ' | sed 's/,$//')
            else
                PR_TITLE="Unknown (GITHUB_TOKEN not set)"
                PR_USER="Unknown"
                PR_LABELS=""
            fi
            
            RELEASE_NOTES="$RELEASE_NOTES

**PR #$PR_NUMBER** by @$PR_USER
- **Title:** $PR_TITLE
- **Commit:** \`$COMMIT_HASH\`
- **Labels:** $PR_LABELS"
        fi
    done <<< "$MERGED_PRS"
else
    RELEASE_NOTES="$RELEASE_NOTES

### Changes in this release

This release includes direct commits and improvements."
fi

# Add standard sections
RELEASE_NOTES="$RELEASE_NOTES

### Downloads

**Linux:**
- [plexify-linux-amd64](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-linux-amd64)
- [plexify-linux-arm64](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-linux-arm64)

**macOS:**
- [plexify-darwin-amd64](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-darwin-amd64)
- [plexify-darwin-arm64](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-darwin-arm64)

**Windows:**
- [plexify-windows-amd64.exe](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-windows-amd64.exe)
- [plexify-windows-arm64.exe](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/releases/download/$VERSION/plexify-windows-arm64.exe)

### Installation

Download the appropriate binary for your platform and make it executable:

\`\`\`bash
# Linux/macOS
chmod +x plexify-<platform>-<arch>

# Windows
# No additional steps needed
\`\`\`

### Usage

See the [README](https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/blob/main/README.md) for detailed usage instructions."

echo "$RELEASE_NOTES" 