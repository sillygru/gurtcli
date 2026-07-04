1: update version on both npm/package.json and version.go from {old_version} to {new_version}
2: read git diff
3: commit with the tag {new_version}
the commit must have a title under 40 chars being a summary of every change done within the git diff
the commit must have a description of every change done
4: push changes with the tag included, tag being {new_version}
