# AMD RAID (RAIDXpert2) SMART on Windows

`smartctl` cannot see drives inside an AMD RAIDXpert2 array. It only sees the
virtual array (`Vendor: AMD-RAID`, "device lacks SMART capability"). The Windows
agent reads the member drives through `AMD_RC2t7x64.dll` by Gakuto Matsumura, the
library CrystalDiskInfo uses.

## Setup

1. Install [CrystalDiskInfo](https://crystalmark.info/en/software/crystaldiskinfo/)
   (or get the DLL from [thilmera.com](https://en.thilmera.com/project/AMD_RC2t7/)).
   CrystalDiskInfo puts it at
   `C:\Program Files\CrystalDiskInfo\CdiResource\dll\AMD_RC2t7x64.dll`.
2. Copy the DLL into a `dll` subfolder of the folder the agent is **launched from**
   (see the table below):

   ```
   <launch folder>\beszel-agent.exe
   <launch folder>\dll\AMD_RC2t7x64.dll
   ```

3. Restart the agent service. Member drives appear in the S.M.A.R.T. tab as
   `amdraid0`, `amdraid1`, …

### Where to put the DLL

The DLL checks its own location. It refuses to initialize (`status=name_failed`)
unless it sits in a subfolder of the path the agent exe was launched from.
Symlink targets do not count, and `AMD_RC2_DLL` cannot override this.

| Install method        | Agent launched from (service `Application`)                     | Put the DLL in                                             |
| --------------------- | --------------------------------------------------------------- | ---------------------------------------------------------- |
| WinGet (default)      | `%LOCALAPPDATA%\Microsoft\WinGet\Links\beszel-agent.exe` (symlink) | `%LOCALAPPDATA%\Microsoft\WinGet\Links\dll\`               |
| Manual / install script | the folder containing `beszel-agent.exe`                        | `<that folder>\dll\`                                       |

For WinGet, the real exe lives in
`%LOCALAPPDATA%\Microsoft\WinGet\Packages\henrygd.beszel-agent_Microsoft.Winget.Source_8wekyb3d8bbwe\`,
but a DLL placed there does **not** work because the service starts the `Links`
symlink. The `Links\dll` folder also survives agent upgrades.

`%LOCALAPPDATA%` is the profile of the user who ran `winget install`, e.g.
`C:\Users\<username>\AppData\Local`, even though the service runs as LocalSystem.

To check the launch path, run this in an elevated PowerShell:

```powershell
(Get-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Services\beszel-agent\Parameters).Application
```

If the AMD RAID driver is present but the DLL is not found, the agent logs the
expected path at INFO level:

```
INFO AMD RAID driver found; copy AMD_RC2t7x64.dll from CrystalDiskInfo here to read member drives path=...
```

## Environment variables

| Variable        | Description                                                                                  |
| --------------- | -------------------------------------------------------------------------------------------- |
| `AMD_RC2_DLL`   | Full path to the DLL. Default: `<launch folder>\dll\AMD_RC2t7x64.dll`. Must still be a subfolder of the launch folder. |
| `EXCLUDE_SMART` | Hide the virtual arrays that report no SMART (e.g. `/dev/sdf,/dev/sdg`) or members such as `amdraid0`. |
| `SMART_DEVICES` | List members manually as `amdraid0:amdraid,amdraid1:amdraid`.                                |

All variables also accept the `BESZEL_AGENT_` prefix.

## Requirements and safety checks

- Windows x64, agent running elevated. The default LocalSystem service is fine.
- The `rcraid.sys` driver must be **9.3 or newer**. Older 9.2.0.x drivers are known
  to bluescreen with NVMe arrays, so the agent skips the DLL on them.
- The DLL must carry a valid Authenticode signature. The publisher is not pinned.
- Without the DLL (or without AMD RAID), the agent works as before. Member drives
  are simply not shown.

## What is collected

- **NVMe:** the SMART / Health log: temperature, critical warning, available
  spare, percentage used, data read/written, power-on hours, unsafe shutdowns,
  media errors, and more. Health is PASSED when the critical warning is 0.
- **SATA:** all ATA SMART attributes with thresholds. Health is FAILED if any
  attribute is at or below its threshold.

## Licensing

`AMD_RC2t7` is closed source and is **not** bundled with Beszel. Its license
(20230618A) forbids redistributing it on its own, so users copy it from their own
CrystalDiskInfo install or the official site.

## Testing

```
go test -run 'AmdRaid|AmdRc2' -v ./agent
```

- `amdraid_common_test.go`: checks the struct size and the NVMe/ATA parsers. Runs on all platforms.
- `TestAmdRaidWithoutDll`: a missing DLL and an unsigned fake DLL both yield no
  devices, and collecting an `amdraid` device returns an error without crashing.
- `TestAmdRaidWithDll`: runs on real hardware. It copies the DLL from CrystalDiskInfo
  (or `AMD_RC2_TEST_DLL`) into `<test exe>\dll\` and reads every member drive.
  It is skipped without the DLL, admin rights, or a 9.3+ RAIDXpert2 driver.

## Implementation

- `agent/amdraid_windows.go`: DLL loading, safety checks, scan and collection.
- `agent/amdraid_common.go`: NVMe/ATA SMART buffer parsers.
- `agent/amdraid_stub.go`: no-op on non-Windows builds.
