import json
import os
from typing import Dict, Any, List

CONFIG_FILE = "config.json"

DEFAULT_CONFIG = {
    "paths": {
        "tun2socks": "",
        "wintun": "",
        "sandboxie_ini": "",
        "sbie_ini_exe": ""
    },
    "proxies_list": [],
    "last_shortcuts_dir": ""
}

class ConfigManager:
    def __init__(self):
        # Config is now in the project root (one level up from manager/)
        root_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        self.config_path = os.path.join(root_dir, CONFIG_FILE)
        self.config = self._load_config()

    def _load_config(self) -> Dict[str, Any]:
        if not os.path.exists(self.config_path):
            return self._create_default_config()
        
        try:
            with open(self.config_path, 'r', encoding='utf-8') as f:
                return json.load(f)
        except (json.JSONDecodeError, IOError):
            return self._create_default_config()

    def _create_default_config(self) -> Dict[str, Any]:
        # Pre-populate defaults if we can find them in parent directory
        base_dir = os.path.dirname(os.path.dirname(__file__)) # vpn-proxy
        tun2socks_guess = os.path.join(base_dir, "tun2socks.exe")
        wintun_guess = os.path.join(base_dir, "wintun.dll")
        
        config = DEFAULT_CONFIG.copy()
        if os.path.exists(tun2socks_guess):
            config["paths"]["tun2socks"] = tun2socks_guess
        if os.path.exists(wintun_guess):
            config["paths"]["wintun"] = wintun_guess
            
        self.save_config(config)
        return config

    def save_config(self, new_config: Dict[str, Any] = None):
        if new_config:
            self.config = new_config
            
        with open(self.config_path, 'w', encoding='utf-8') as f:
            json.dump(self.config, f, indent=4)

    def get(self, key: str, default=None) -> Any:
        return self.config.get(key, default)

    def set(self, key: str, value: Any):
        self.config[key] = value
        self.save_config()

    def get_path(self, name: str) -> str:
        return self.config["paths"].get(name, "")

    def set_path(self, name: str, path: str):
        self.config["paths"][name] = path
        self.save_config()
