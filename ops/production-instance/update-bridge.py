#!/usr/bin/env python3
"""Fixed production adapter for the reviewed update-control protocol."""
import importlib.util,pathlib
spec=importlib.util.spec_from_file_location("production_update_bridge","/opt/dob-production/bin/project-update.py")
bridge=importlib.util.module_from_spec(spec);spec.loader.exec_module(bridge)
bridge.STATE=pathlib.Path("/var/lib/dob-production-updater")
bridge.RUNTIME=pathlib.Path("/run/dob-production")
bridge.MANIFEST=pathlib.Path("/opt/dob-production/build-manifest.json")
bridge.PENDING=pathlib.Path("/etc/dob-production/deployment.pending")
bridge.INSTALLER_LOCK="/run/lock/dob-production-installer.lock"
bridge.UPDATE_UNIT="dob-production-update.service"
bridge.RESTART_UNIT="dob-production-restart-api.service"
bridge.UPDATE_COMMAND=["/usr/local/sbin/dob-production","update"]
bridge.API_UNIT="dob-production-api"
bridge.DEPLOY_LOCK="/opt/.dob-production-deploy.lock"
if __name__=="__main__":bridge.main()
