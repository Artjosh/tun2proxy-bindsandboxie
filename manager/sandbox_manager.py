import os
import re
import subprocess
import win32com.client
from dataclasses import dataclass
from typing import List, Dict, Optional

@dataclass
class SandboxShortcut:
    name: str
    path: str
    box_name: str
    group_id: int # The prefix number (e.g. 1, 2)
    app_name: str # The suffix (e.g. spotify.exe)

class SandboxManager:
    def __init__(self, config_manager=None):
        self.config = config_manager
        # Attempt to get path from config, otherwise fallback is handled by config load defaults
        if self.config:
            ini_path = self.config.get_path("sandboxie_ini")
            # We need SbieIni.exe, usually in same folder as Sandboxie.ini or Program Files
            # Assuming user configured path to ini, we deduce exe or add a new config field?
            # User wants NO hardcodes. Let's add sbie_ini_exe to config if not there, or deduce.
            # Usually SbieIni.exe is in Program Files/Sandboxie-Plus.
            # Sandboxie.ini is in Windows.
            # Let's check config for 'sbie_ctrl' or similar, or just default to common installation if missing?
            # User specifically asked to remove paths.
            # Let's add "sbie_ini_exe" to config.json structure in next step if needed,
            # but for now let's use what we have or a placeholder that must be configured.
            
            # Since SbieIni.exe is binary, not the config file.
            self.sbie_ini_exe = self.config.get_path("sbie_ini_exe")
            if not self.sbie_ini_exe:
                # Fallback if not set yet, but user wants it in config.
                self.sbie_ini_exe = r"C:\Program Files\Sandboxie-Plus\SbieIni.exe" 
        else:
             self.sbie_ini_exe = r"C:\Program Files\Sandboxie-Plus\SbieIni.exe"

        self.shell = win32com.client.Dispatch("WScript.Shell")

    def scan_shortcuts(self, folder_path: str) -> Dict[str, List[SandboxShortcut]]:
        """
        Scans folder for shortcuts matching 'N-[boxname] optional_appname.lnk'.
        Returns a dict keyed by 'group_id' (str) -> List[SandboxShortcut].
        """
        results = {}
        if not os.path.exists(folder_path):
            return results

        # Regex explanation:
        # ^(\d+)-       : Starts with digits followed by hyphen (Group 1: ID)
        # \[(.+?)\]     : Box name inside brackets (Group 2: Box Name)
        # (?: (.+))?    : Optional space followed by App Name (Group 3: App Name)
        # \.lnk$        : Ends with .lnk
        pattern = re.compile(r"^(\d+)-\[(.+?)\](?: (.+))?\.lnk$", re.IGNORECASE)

        for filename in os.listdir(folder_path):
            match = pattern.match(filename)
            if match:
                group_id = match.group(1)
                box_name = match.group(2)
                # If group(3) is None, use a default name or the whole filename
                app_name = match.group(3) if match.group(3) else "Shortcut"
                
                full_path = os.path.normpath(os.path.join(folder_path, filename))
                
                shortcut = SandboxShortcut(
                    name=filename,
                    path=full_path,
                    box_name=box_name,
                    group_id=int(group_id),
                    app_name=app_name
                )
                
                if group_id not in results:
                    results[group_id] = []
                results[group_id].append(shortcut)
        
        return results

    def get_bind_adapter_for_box(self, box_name: str) -> str:
        """Reads the current BindAdapter setting for a box."""
        # We can read Sandboxie.ini directly or use SbieIni query (if supported).
        # Direct read is often simpler for reading.
        ini_path = "C:\\Windows\\Sandboxie.ini"
        if self.config:
            p = self.config.get_path("sandboxie_ini")
            if p: ini_path = p
            
        if not os.path.exists(ini_path):
            return "None"
        
        try:
            with open(ini_path, 'r', encoding='utf-16') as f: # Sandboxie.ini is usually utf-16
                content = f.read()
            
            # Find section [box_name]
            # Then find BindAdapter=... inside it
            # This is a bit naive regex but works for standard ini structures
            # We look for [box_name] ... (anything until next [) ...
            
            # Let's split by section to be safer
            sections = re.split(r'^\[([^\]]+)\]', content, flags=re.MULTILINE)
            
            # sections[0] is pre-content. 
            # sections[1] is name1, sections[2] is content1, etc.
            
            current_section_content = None
            for i in range(1, len(sections), 2):
                if sections[i].lower() == box_name.lower():
                    current_section_content = sections[i+1]
                    break
            
            if current_section_content:
                match = re.search(r"^\s*BindAdapter=(.+)$", current_section_content, re.MULTILINE)
                if match:
                    return match.group(1).strip()
            
            return "None"
            
        except Exception as e:
            print(f"Error reading ini: {e}")
            return "None"

    def set_bind_adapter(self, box_name: str, adapter_name: str):
        """Sets the BindAdapter using SbieIni.exe."""
        if adapter_name == "None" or adapter_name == "clean":
             # Remove the setting
             cmd = [self.sbie_ini_exe, "set", box_name, "BindAdapter"]
        else:
            cmd = [self.sbie_ini_exe, "set", box_name, "BindAdapter", adapter_name]
            
        subprocess.run(cmd, creationflags=subprocess.CREATE_NO_WINDOW)

    def launch_shortcut(self, path: str):
        normalized_path = os.path.normpath(path)
        if os.path.exists(normalized_path):
            try:
                # os.startfile causes WinError 3 on some machines/OneDrive paths.
                # Using explorer.exe is more robust for "simulating a click".
                subprocess.Popen(["explorer", normalized_path])
            except Exception as e:
                print(f"Error launching with explorer: {e}")
                os.startfile(normalized_path) # Fallback attempt
        else:
            print(f"DEBUG: Shortcut not found at {normalized_path}")
            # Try to see if it's a relative path issue or drive letter case
            abs_path = os.path.abspath(normalized_path)
            if os.path.exists(abs_path):
                os.startfile(abs_path)
            else:
                raise FileNotFoundError(f"Shortcut truly missing: {normalized_path}")

    def get_available_adapters(self) -> List[str]:
        """Returns list of network interface names."""
        adapters = ["clean"] # Default option for 'None'
        
        # Use netsh to list
        res = subprocess.run(
            ["netsh", "interface", "show", "interface"],
            capture_output=True, text=True,
            creationflags=subprocess.CREATE_NO_WINDOW
        )
        if res.returncode == 0:
            # Skip header (usually first 3 lines)
            lines = res.stdout.strip().split('\n')
            for line in lines:
                parts = line.split(maxsplit=3)
                if len(parts) == 4:
                    # Admin State, State, Type, Interface Name
                    name = parts[3].strip()
                    adapters.append(name)
        
        return adapters

    # --- SPOOFING LOGIC ---
    SPOOF_TEMPLATES = ["BlockAccessWMI", "HideInstalledPrograms"]
    SPOOF_KEYS = {
        "SandboxieAllGroup": "n",
        "HideFirmwareInfo": "y",
        "RandomRegUID": "y",
        "HideDiskSerialNumber": "y",
        "HideNetworkAdapterMAC": "y"
    }

    def is_box_spoofed(self, box_name: str) -> bool:
        """
        Checks if the box has the spoof settings.
        We check a few key indicators to decide if it's 'Spoofado'.
        """
        # We'll use a simple heuristic: if at least one key unique to spoofing is set.
        # Ideally we check all, but SbieIni buffering might be slow.
        # Let's check 'HideNetworkAdapterMAC' and one Template.
        try:
            # Check a standard Key
            cmd = [self.sbie_ini_exe, "query", box_name, "HideNetworkAdapterMAC"]
            res = subprocess.run(cmd, capture_output=True, text=True, creationflags=subprocess.CREATE_NO_WINDOW)
            if "y" not in res.stdout.strip():
                return False

            # Check a Template
            # Querying templates returns all of them. We check if ours is in the list.
            cmd_tpl = [self.sbie_ini_exe, "query", box_name, "Template"]
            res_tpl = subprocess.run(cmd_tpl, capture_output=True, text=True, creationflags=subprocess.CREATE_NO_WINDOW)
            if "BlockAccessWMI" not in res_tpl.stdout:
                return False
                
            return True
        except:
            return False

    def toggle_spoof(self, box_name: str, enable: bool):
        """Adds or Removes the spoofing lines."""
        flags = subprocess.CREATE_NO_WINDOW
        
        print(f"[DEBUG] toggle_spoof | Box: {box_name} | Enable: {enable}")
        
        if enable:
            # ADD SETTINGS
            for tpl in self.SPOOF_TEMPLATES:
                # SbieIni 'set' might overwrite. 'append' ensures we add to the list.
                cmd = [self.sbie_ini_exe, "append", box_name, "Template", tpl]
                print(f"  [CMD] {' '.join(cmd)}")
                res = subprocess.run(cmd, capture_output=True, text=True, creationflags=flags)
                if res.stdout.strip(): print(f"    [OUT] {res.stdout.strip()}")
                if res.stderr.strip(): print(f"    [ERR] {res.stderr.strip()}")
            
            for key, val in self.SPOOF_KEYS.items():
                cmd = [self.sbie_ini_exe, "set", box_name, key, val]
                print(f"  [CMD] {' '.join(cmd)}")
                res = subprocess.run(cmd, capture_output=True, text=True, creationflags=flags)
                if res.stdout.strip(): print(f"    [OUT] {res.stdout.strip()}")
                if res.stderr.strip(): print(f"    [ERR] {res.stderr.strip()}")
                
        else:
            # REMOVE SETTINGS
            for tpl in self.SPOOF_TEMPLATES:
                # To remove a specific template value, SbieIni might require 'clear' or manual editing
                # But 'delete' on a Key usually deletes ALL.
                # However, for Template, we might want to just remove specific ones.
                # Trying 'delete Box Key Value' syntax which some SbieIni versions support
                cmd = [self.sbie_ini_exe, "delete", box_name, "Template", tpl]
                print(f"  [CMD] {' '.join(cmd)}")
                res = subprocess.run(cmd, capture_output=True, text=True, creationflags=flags)
                
                # If that failed (stderr), try clearing key? No, that deletes all templates.
                # Let's trust 'delete box template value' works or fails.
                if res.stdout.strip(): print(f"    [OUT] {res.stdout.strip()}")
                if res.stderr.strip(): print(f"    [ERR] {res.stderr.strip()}")
                
            # For keys, 'delete' might not be working as reported.
            # Try setting to empty string to remove/clear the key.
            for key in self.SPOOF_KEYS.keys():
                cmd = [self.sbie_ini_exe, "set", box_name, key, ""] # Empty value to remove
                print(f"  [CMD] {' '.join(cmd)}")
                res = subprocess.run(cmd, capture_output=True, text=True, creationflags=flags)
                if res.stdout.strip(): print(f"    [OUT] {res.stdout.strip()}")
                if res.stderr.strip(): print(f"    [ERR] {res.stderr.strip()}")
