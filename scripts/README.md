# Installation Scripts

This directory contains installation and update scripts for Git Project Sync across different platforms.

## Installation Scripts

### Unix-like Systems (Linux/macOS)

**Quick Install:**
```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
```

**Custom Install Directory:**
```bash
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
```

**What it does:**
- Detects your OS (Linux or macOS) and architecture (x86_64 or aarch64)
- Downloads the latest release binary from GitHub
- Installs to `~/.local/bin/mirror-cli` by default
- Makes the binary executable
- Provides instructions for adding to PATH if needed

### Windows (PowerShell)

**Quick Install:**
```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 | iex
```

**Install with PATH Update:**
```powershell
iwr -Uri https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -AddToPath
```

**Custom Install Directory:**
```powershell
iwr -Uri https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -InstallDir "C:\Tools\mirror-cli"
```

**What it does:**
- Detects your architecture (x86_64)
- Downloads the latest release binary from GitHub
- Installs to `%LOCALAPPDATA%\Programs\mirror-cli` by default
- Optionally adds to user PATH (with `-AddToPath` flag)
- Verifies the installation

## Update Scripts

### Unix-like Systems (Linux/macOS)

**Quick Update:**
```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.sh | bash
```

**What it does:**
- Checks your current installed version
- Fetches the latest release version
- Downloads and updates if a newer version is available
- Preserves your installation location
- Notifies if already up to date

### Windows (PowerShell)

**Quick Update:**
```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.ps1 | iex
```

**What it does:**
- Checks your current installed version
- Fetches the latest release version
- Downloads and updates if a newer version is available
- Preserves your installation location
- Notifies if already up to date

## Security Considerations

### Script Source Verification

All scripts:
- Download binaries only from official GitHub releases
- Use HTTPS for all downloads
- Verify download success before installation

### Best Practices

1. **Review Before Running:** It's always a good practice to review scripts before piping to bash/iex:
   ```bash
   # Unix
   curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh -o install.sh
   less install.sh
   bash install.sh
   ```
   
   ```powershell
   # Windows
   Invoke-WebRequest -Uri https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 -OutFile install.ps1
   notepad install.ps1
   .\install.ps1
   ```

2. **Verify Release Integrity:** The scripts download from GitHub releases. You can verify releases at:
   https://github.com/basmulder03/git-project-sync/releases

3. **Use Specific Versions:** While the scripts always fetch the latest version, you can modify them to download specific versions by changing the release URL.

## Troubleshooting

### Unix/Linux/macOS

**Script fails with "command not found":**
- Ensure `curl` is installed: `which curl`
- Ensure `bash` is available: `which bash`

**Binary not in PATH:**
- Add `~/.local/bin` to your PATH:
  ```bash
  echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
  source ~/.bashrc
  ```

**Permission denied:**
- Check the binary is executable: `ls -l ~/.local/bin/mirror-cli`
- Make executable if needed: `chmod +x ~/.local/bin/mirror-cli`

### Windows

**PowerShell execution policy error:**
```powershell
Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned
```

**Binary not in PATH:**
- Manually add to PATH via System Properties → Environment Variables
- Or use `-AddToPath` flag during installation

**Download fails:**
- Check internet connection
- Verify you can access https://github.com
- Try downloading manually from releases page

## Alternative Installation Methods

If the scripts don't work for your setup, you can:

1. **Download manually** from [GitHub Releases](https://github.com/basmulder03/git-project-sync/releases)
2. **Build from source**: `cargo build --release`
3. **Use the built-in installer**: `mirror-cli install` (after building from source)

## Contributing

If you find issues with these scripts or want to add support for additional platforms:

1. Test your changes locally
2. Ensure backward compatibility
3. Update this README with any new features
4. Submit a pull request

## Script Maintenance

These scripts are maintained as part of the Git Project Sync project. For issues or feature requests, please open an issue at:
https://github.com/basmulder03/git-project-sync/issues
