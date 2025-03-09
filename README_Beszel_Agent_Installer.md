❗️I am currently working on an english version of the software.❗️

# Beszel Agent Installer

You need beszel installed for this software. You'll find beszel here: https://github.com/henrygd/beszel

## 🔍 Overview
The **Beszel Agent Installer** is a Windows installation program that installs or removes the Beszel Agent on a system. The installer can optionally create a firewall rule for communication and register the agent as a Windows service using **NSSM (Non-Sucking Service Manager)**.

Big thanks to Alex for providing a tutorial on how to create the agent.exe. Check out his blog here: https://blog.ktz.me/using-beszel-to-monitor-windows/amp/

## 🚀 Features
- **Installs the Beszel Agent** in `C:\Program Files\beszel-agent` (or `C:\Programme\beszel-agent` on German systems)
- **Optional creation of a firewall rule** for port **45876**
- **Registers as a Windows service** using **NSSM**
- **Allows user key input** for configuration
- **Uninstallation of the Beszel Agent**
  - Stops and removes the service
  - Deletes the installation directory
- **Graphical installation wizard with progress bar**
- **Log file for troubleshooting** (`install.log`)

## 🛠️ Installation & Usage

### **1️⃣ Requirements**
- Windows 10 or 11 (64-bit)
- Administrator privileges
- **ATTENTION**: You probably have to deactivate your antivirus as my software is not signed!
- Chocolatey (`choco`) must be installed (automatically installed if necessary)

### **2️⃣ Installation**
1. **Download the installer files** (`installer.exe` + `agent.exe`).
2. **Run `installer.exe`** with **administrator privileges**.
3. **Follow the installation steps** in the wizard:
   - Accept the license agreement
   - Choose between installation or uninstallation
   - Optionally create a firewall rule
   - Enter the public key you got from beszel (Add system --> choose binary)
4. **Click "Install"** and wait for the process to complete.
5. **Check if the service is running:**
   ```sh
   sc query beszelagent
   ```
   If `RUNNING` or `Wird ausgeführt` appears, the installation was successful.

### **3️⃣ Uninstallation**
1. **Run `installer.exe`** with **administrator privileges**.
2. **Select "Uninstall"**.
3. The installer:
   - Stops and removes the **Beszel Agent service**.
   - Deletes the directory `C:\Program Files\beszel-agent\`.

## 🔧 Troubleshooting
If the installer does not work correctly, check the **log file**:

📄 **Log file location:**  
`C:\Program Files\beszel-agent\install.log`

### **1️⃣ Chocolatey is not recognized**
If Chocolatey is not found, try installing it manually:
```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force; `
[System.Net.ServicePointManager]::SecurityProtocol = `
[System.Net.ServicePointManager]::SecurityProtocol -bor 3072; `
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
```

### **2️⃣ NSSM installation fails**
If NSSM is not recognized, install it manually:
```sh
choco install nssm -y
```
Check its installation path:
```sh
where nssm
```
It should return `C:\ProgramData\chocolatey\bin\nssm.exe`.

### **3️⃣ Service does not start**
If the service is not running, try:
```sh
sc query beszelagent
net start beszelagent
```
If that doesn’t work, remove and reinstall the service:
```sh
nssm remove beszelagent confirm
nssm install beszelagent "C:\Program Files\beszel-agent\agent.exe"
nssm start beszelagent
```

## 💻 Development
If you want to modify the installer:

### **1️⃣ Requirements**
- Python 3.9 or later
- Tkinter (GUI library)
- PyInstaller (to create the `.exe` file)

### **2️⃣ Creating an executable file (`installer.exe`)**
If you want to compile the installer yourself:
```sh
pyinstaller --onefile --windowed --icon=installer.ico installer.py
```
📌 **Note:** Replace `"installer.ico"` with your own icon.

## 📝 License
This project is licensed under the **MIT License**. See the [`LICENSE`](LICENSE) file for details.

## 🤝 Contributing
Contributions are always welcome! If you find bugs or want to suggest new features:
1. **Fork the repository**.
2. **Create a feature branch**:
   ```sh
   git checkout -b feature-new-functionality
   ```
3. **Make your changes**.
4. **Submit a pull request**.

## 📞 Support
If you have questions or issues, open a **GitHub issue** or contact me directly.

---

📌 **Created by:**  
**Marko Buculovic - VMHOMELAB** 🚀  
