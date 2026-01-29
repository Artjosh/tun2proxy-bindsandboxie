import subprocess
import time
import re
import threading
from dataclasses import dataclass
from typing import List, Callable

@dataclass
class ProxyConfig:
    ip: str
    port: str
    username: str
    password: str
    id: int

class ProxyEngine:
    def __init__(self, tun2socks_path: str):
        self.tun2socks_path = tun2socks_path
        self._stop_flag = False

    def parse_proxies(self, text: str) -> List[ProxyConfig]:
        """Parses lines of IP:PORT:USER:PASS into ProxyConfig objects."""
        proxies = []
        lines = text.strip().split('\n')
        for i, line in enumerate(lines):
            parts = line.strip().split(':')
            if len(parts) >= 4:
                proxies.append(ProxyConfig(
                    ip=parts[0],
                    port=parts[1],
                    username=parts[2],
                    password=parts[3],
                    id=i + 1
                ))
        return proxies

    def stop_all(self):
        """Kills all tun2socks processes."""
        subprocess.run(["taskkill", "/F", "/IM", "tun2socks.exe"], 
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def start_proxies(self, proxies: List[ProxyConfig], log_callback: Callable[[str], None] = None):
        """Starts a list of proxies sequentially."""
        self.stop_all()
        
        def log(msg):
            if log_callback:
                log_callback(msg)
            else:
                print(msg)

        # Threading this to not freeze GUI
        threading.Thread(target=self._run_sequence, args=(proxies, log), daemon=True).start()

    def _run_sequence(self, proxies: List[ProxyConfig], log):
        for p in proxies:
            dev_name = f"Proxy_{p.id}"
            
            # Start tun2socks
            cmd = [
                self.tun2socks_path,
                "-device", dev_name,
                "-proxy", f"socks5://{p.username}:{p.password}@{p.ip}:{p.port}",
                "-loglevel", "error" # Reduce noise
            ]
            
            log(f"[{p.id}] Starting interface {dev_name} connected to {p.ip}...")
            # Use specific flags to hide window if possible, but subprocess.Popen is standard
            subprocess.Popen(cmd, creationflags=subprocess.CREATE_NO_WINDOW)
            
            # Wait for interface
            if not self._wait_for_interface(dev_name):
                 log(f"[{p.id}] ERROR: Interface {dev_name} failed to appear.")
                 continue

            # Configure IP
            local_ip = f"10.0.{p.id}.1"
            gateway = f"10.0.{p.id}.254"
            log(f"[{p.id}] Setting IP {local_ip}...")
            
            self._set_ip(dev_name, local_ip, gateway)
            log(f"[{p.id}] Ready.")

        log("All proxies started.")

    def _wait_for_interface(self, name: str, timeout: int = 10) -> bool:
        """Polls netsh to see if interface exists."""
        start = time.time()
        while time.time() - start < timeout:
            res = subprocess.run(
                ["netsh", "interface", "show", "interface", f"name={name}"],
                capture_output=True, text=True
            )
            if res.returncode == 0:
                return True
            time.sleep(0.5)
        return False

    def _set_ip(self, name: str, ip: str, gateway: str):
        # set address
        subprocess.run([
            "netsh", "interface", "ip", "set", "address", 
            f"name={name}", "source=static", f"addr={ip}", 
            "mask=255.255.255.0", f"gateway={gateway}"
        ], stdout=subprocess.DEVNULL, creationflags=subprocess.CREATE_NO_WINDOW)
        
        # set metric
        subprocess.run([
            "netsh", "interface", "ip", "set", "interface", 
            name, "metric=500"
        ], stdout=subprocess.DEVNULL, creationflags=subprocess.CREATE_NO_WINDOW)
