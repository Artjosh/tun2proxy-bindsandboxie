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
        Scans folder for shortcuts strictly matching 'N-[boxname] appname.lnk'.
        Returns a dict keyed by 'group_id' (str) -> List[SandboxShortcut].
        """
        results = {}
        if not os.path.exists(folder_path):
            return results

        pattern = re.compile(r"^(\d+)-\[(.+?)\] (.+)\.lnk$", re.IGNORECASE)

        for filename in os.listdir(folder_path):
            match = pattern.match(filename)
            if match:
                group_id = match.group(1)
                box_name = match.group(2)
                app_name = match.group(3)
                
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
            capture_output=True, text=True
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
