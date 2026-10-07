# VaultChron on Windows

Windows (amd64) is supported. There are no arm64 builds yet. The programs are not code-signed.

You need Git for Windows on your PATH and an LLM API key. You do not need Go.

## Install

Either use the installer:

```powershell
irm https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/install.ps1 | iex
```

It installs to `%LOCALAPPDATA%\Programs\vaultchron` and adds that folder to your user PATH. Open a new terminal afterwards. Set `VAULTCHRON_INSTALL_DIR` first to choose another folder.

Or download `vaultchron_<version>_windows_amd64.zip` and `checksums.txt` from the [Releases page](https://github.com/ZeezyCodes/vaultchron/releases). Compare the zip's hash with its line in `checksums.txt`:

```powershell
(Get-FileHash .\vaultchron_<version>_windows_amd64.zip -Algorithm SHA256).Hash
```

Then extract the zip. It holds `vaultchron.exe`, `vaultchron_migrate.exe`, `config.example.yaml` and the scheduling scripts in `deploy\windows\`. You need the zip folder if you want the Scheduled Task below.

## Downloads from a browser

If you downloaded the zip in a browser, Windows may block it. Unblock it before extracting:

```powershell
Unblock-File -Path .\vaultchron_*_windows_amd64.zip
```

The first time you run the programs, SmartScreen may show a warning because they are unsigned. Click **More info**, then **Run anyway**.

## Configure

For manual runs, VaultChron looks for `.\config.yaml`, then `%APPDATA%\vaultchron\config.yaml`:

```powershell
New-Item -ItemType Directory -Force "$env:APPDATA\vaultchron"
irm https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/config.example.yaml -OutFile "$env:APPDATA\vaultchron\config.yaml"
```

The Scheduled Task is different. It always uses `config.yaml` in the program folder, next to `vaultchron.exe`, and ignores the `%APPDATA%` file. If you use the task, also run this from the extracted zip folder:

```powershell
Copy-Item .\config.example.yaml .\config.yaml
```

Set `vault.path` and `scan.roots` in the file. In YAML, write Windows paths with forward slashes (`C:/Users/me/vault`) or in single quotes (`'C:\Users\me\vault'`). `~` and `$HOME` expand to your user profile folder. All other keys are in [configuration.md](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/configuration.md).

## API key

Set it for your user account (it applies to new shells):

```powershell
[Environment]::SetEnvironmentVariable('GOOGLE_API_KEY', 'your-key', 'User')
```

Or for the current shell only:

```powershell
$env:GOOGLE_API_KEY = 'your-key'
```

You can also create `%APPDATA%\vaultchron\env` with `KEY=value` lines (lines starting with `#` are ignored, and quotes around a value are removed):

```text
GOOGLE_API_KEY=your-key
```

Only the wrapper script `vaultchron-run.ps1`, and therefore the Scheduled Task, reads that file. Manual runs need the user variable or the session variable.

## Run by hand

```powershell
.\vaultchron.exe -scan
.\vaultchron.exe -dry-run
.\vaultchron.exe
```

## Run every day with a Scheduled Task

From the extracted zip folder:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1
```

Options:

- `-Time`: daily start time as 24-hour `HH:mm`. Default `07:00`.
- `-TaskName`: name of the task. Default `VaultChron`.
- `-Unregister`: remove the task.

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1 -Unregister
```

Scripts from a downloaded zip can fail with "is not digitally signed". The command above avoids that for this one run. You can also unblock the folder once with `Get-ChildItem -Recurse . | Unblock-File`.

The task runs as your user with interactive logon, so it runs only while you are logged on. It needs no administrator rights and stores no password. If the PC was off at start time, the task runs when it can (`StartWhenAvailable`). It may run on battery. A single run is stopped after 2 hours. Registering again with the same name replaces the task.

The task remembers the full path of the zip folder. If you move the folder, register the task again.

To run it now or check the last result:

```powershell
Start-ScheduledTask -TaskName 'VaultChron'
Get-ScheduledTaskInfo -TaskName 'VaultChron'
```

## How the wrapper finds the program

`vaultchron-run.ps1` looks for `vaultchron.exe` in this order: the folder in the `VAULTCHRON_HOME` environment variable, the zip folder (two levels above the script), then `%LOCALAPPDATA%\vaultchron`. It then runs `vaultchron.exe -config <that folder>\config.yaml`.

## Logs

The wrapper and the Scheduled Task write a log to `%LOCALAPPDATA%\vaultchron\logs\vaultchron.log`. Each run has a start line and a finish line with the exit code.
