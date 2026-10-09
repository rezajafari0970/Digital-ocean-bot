#!/usr/bin/env bash
# Shared desired topology for fresh install and upgrade.
SERVICES=(digital-ocean-bot-vultr-browser-manager digital-ocean-bot-api digital-ocean-bot-worker digital-ocean-bot-worker-panels)
DOB_UNIT_DIR=${DOB_UNIT_DIR:-/etc/systemd/system}
ETC=${ETC:-/etc/digital-ocean-bot}
DOB_UNIT_ASSETS=(
 digital-ocean-bot-api.service
 digital-ocean-bot-worker.service
 digital-ocean-bot-worker-panels.service
 digital-ocean-bot-vultr-browser-manager.service
 digital-ocean-bot-api.service.d/60-worker-isolation.conf
 digital-ocean-bot-worker.service.d/60-worker-isolation.conf
)
dob_preflight_service_topology() {
 local unit path
 for unit in "${SERVICES[@]}"; do
  for path in "$DOB_UNIT_DIR/$unit.service.d/"*.conf $(systemctl show "$unit" -p DropInPaths --value); do
   [ -f "$path" ] || continue
   case "$path" in
    "$DOB_UNIT_DIR/digital-ocean-bot-api.service.d/60-worker-isolation.conf"|"$DOB_UNIT_DIR/digital-ocean-bot-worker.service.d/60-worker-isolation.conf") continue ;;
   esac
   if grep -Eq '^[[:space:]]*(ExecStart|Type|WatchdogSec|WatchdogSignal|NotifyAccess|EnvironmentFile)[[:space:]]*=|^[[:space:]]*Environment[[:space:]]*=([[:space:]]*$|.*DOB_WORKER_MODE)|^[[:space:]]*UnsetEnvironment[[:space:]]*=.*DOB_WORKER_MODE' "$path"; then
    echo "Conflicting unowned service topology override: $path. Resolve this override before installation; existing services are unchanged." >&2
    return 1
   fi
  done
 done
}
# One publisher owns the host from staging through rollback/process exit.
dob_acquire_deploy_lock() {
 exec {DOB_DEPLOY_LOCK_FD}>"$(dirname "$APP")/.digital-ocean-bot-deploy.lock"
 if ! flock -n "$DOB_DEPLOY_LOCK_FD"; then
  echo "Another deployment owns the runtime; no changes made." >&2
  return 1
 fi
}
dob_stop_runtime_readers() {
 local unit load active pid
 for unit in "${SERVICES[@]}"; do
  load=$(systemctl show "$unit" -p LoadState --value) || return
  case "$load" in
   not-found) continue ;;
   loaded) ;;
   *) echo "Cannot establish service load state for $unit: $load" >&2; return 1 ;;
  esac
  systemctl stop "$unit" || return
  active=$(systemctl show "$unit" -p ActiveState --value) || return
  pid=$(systemctl show "$unit" -p MainPID --value) || return
  test "$pid" = 0 || return
  case "$active" in inactive|failed) ;; *) return 1 ;; esac
 done
}
dob_verify_exact_exec() {
 local unit=$1 expected=$2 actual
 actual=$(systemctl show "$unit" -p ExecStart --value)
 actual=${actual#*argv[]=}
 actual=${actual%% ;*}
 test "$actual" = "$expected"
}
dob_install_service_topology() {
 local unit
 for unit in "${DOB_UNIT_ASSETS[@]:0:4}"; do
   install -m 0644 "$SRC/deploy/$unit" "$DOB_UNIT_DIR/$unit"
 done
 install -d -m 0755 "$DOB_UNIT_DIR/digital-ocean-bot-api.service.d" "$DOB_UNIT_DIR/digital-ocean-bot-worker.service.d"
 install -m 0644 "$SRC/deploy/api-split-workers.conf" "$DOB_UNIT_DIR/digital-ocean-bot-api.service.d/60-worker-isolation.conf"
 install -m 0644 "$SRC/deploy/worker-control.conf" "$DOB_UNIT_DIR/digital-ocean-bot-worker.service.d/60-worker-isolation.conf"
 systemctl daemon-reload
}
# Prepare every artifact outside live paths before stopping services.
dob_cleanup_stage() {
 if [ -n "${DOB_STAGE:-}" ] && [ -d "$DOB_STAGE" ]; then rm -rf -- "$DOB_STAGE"; fi
}
dob_stage_runtime() {
 DOB_STAGE=$(mktemp -d "$(dirname "$APP")/.dob-release.XXXXXXXX")
 trap dob_cleanup_stage EXIT
 install -d -m 0755 "$DOB_STAGE/bin" "$DOB_STAGE/web/static"
 local binary command arch api_sha worker_sha guardian_amd64_sha guardian_arm64_sha
 for binary in digital-ocean-bot-api digital-ocean-bot-worker vultr-browser-session vultr-browser-manager vultr-input-bridge; do
  command=$binary
  case "$binary" in digital-ocean-bot-api) command=api ;; digital-ocean-bot-worker) command=worker ;; esac
  (cd "$SRC" && go build -trimpath -ldflags="$LDFLAGS" -o "$DOB_STAGE/bin/$binary" "./cmd/$command")
 done
 for arch in amd64 arm64; do
  (cd "$SRC" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="$LDFLAGS" -o "$DOB_STAGE/bin/server-guardian-linux-$arch" ./cmd/server-guardian)
 done
 cp -a "$SRC/migrations" "$DOB_STAGE/"
 cp -a "$SRC/web/static/." "$DOB_STAGE/web/static/"
 api_sha=$(sha256sum "$DOB_STAGE/bin/digital-ocean-bot-api" | awk '{print $1}')
 worker_sha=$(sha256sum "$DOB_STAGE/bin/digital-ocean-bot-worker" | awk '{print $1}')
 guardian_amd64_sha=$(sha256sum "$DOB_STAGE/bin/server-guardian-linux-amd64" | awk '{print $1}')
 guardian_arm64_sha=$(sha256sum "$DOB_STAGE/bin/server-guardian-linux-arm64" | awk '{print $1}')
 printf '{"residential_admission_schema":1,"commit":"%s","build_time":"%s","api_sha256":"%s","worker_sha256":"%s","guardian_sha256":{"amd64":"%s","arm64":"%s"}}\n' "$BUILD_COMMIT" "$BUILD_TIME" "$api_sha" "$worker_sha" "$guardian_amd64_sha" "$guardian_arm64_sha" > "$DOB_STAGE/build-manifest.json"
 dob_verify_staged_runtime
}
# Both install and upgrade must reject an incomplete release before stopping readers.
dob_verify_staged_runtime() {
 python3 - "$DOB_STAGE" <<'PY'
import hashlib,json,pathlib,struct,sys
root=pathlib.Path(sys.argv[1]);manifest=json.loads((root/'build-manifest.json').read_text())
for name in ['digital-ocean-bot-api','digital-ocean-bot-worker','vultr-browser-session','vultr-browser-manager','vultr-input-bridge','server-guardian-linux-amd64','server-guardian-linux-arm64']:
 p=root/'bin'/name
 if not (p.is_file() and not p.is_symlink() and p.stat().st_size>0):raise SystemExit('required artifact missing: '+name)
for name,key in [('digital-ocean-bot-api','api_sha256'),('digital-ocean-bot-worker','worker_sha256')]:
 if hashlib.sha256((root/'bin'/name).read_bytes()).hexdigest()!=manifest[key]:raise SystemExit('runtime hash mismatch: '+name)
for arch,machine in [('amd64',62),('arm64',183)]:
 raw=(root/'bin'/('server-guardian-linux-'+arch)).read_bytes()
 if not (len(raw)>=20 and raw[:6]==b'\x7fELF\x02\x01' and struct.unpack_from('<H',raw,18)[0]==machine):raise SystemExit('guardian architecture mismatch: '+arch)
 if hashlib.sha256(raw).hexdigest()!=manifest['guardian_sha256'][arch]:raise SystemExit('guardian hash mismatch: '+arch)
print('Verified API, workers and both guardian release artifacts')
PY
}
dob_prepare_stage_permissions() {
 chown -R root:digitaloceanbot "$DOB_STAGE"
 chmod 0750 "$DOB_STAGE" "$DOB_STAGE/bin" "$DOB_STAGE/migrations"
 chmod 0750 "$DOB_STAGE/bin/"*
 chmod 0640 "$DOB_STAGE/migrations/"*.sql
 local migration
 for migration in "$DOB_STAGE/migrations/"*.sql; do runuser -u digitaloceanbot -- test -r "$migration"; done
}
dob_publish_runtime() {
 # All readers are stopped, backup is complete and rollback is armed.
 python3 "$SRC/deploy/admission-compatibility.py" "$SRC/deploy/bootstrap.py" "$ETC/env" "$DOB_STAGE/build-manifest.json" || return
 local path
 install -d "$APP" "$APP/web"
 for path in bin migrations web/static build-manifest.json; do
  rm -rf -- "$APP/$path"
  mv "$DOB_STAGE/$path" "$APP/$path"
 done
}
# Schema remains additive; matching migration files are restored on rollback.

dob_backup_runtime() {
 DOB_BACKUP=$(mktemp -d "$APP.rollback.XXXXXXXX")
 install -d -m 0700 "$DOB_BACKUP/units" "$DOB_BACKUP/runtime"
 local path unit
 for path in "${DOB_UNIT_ASSETS[@]}"; do
  if [ -e "$DOB_UNIT_DIR/$path" ]; then
   install -d "$DOB_BACKUP/units/$(dirname "$path")"
   cp -a "$DOB_UNIT_DIR/$path" "$DOB_BACKUP/units/$path"
  fi
 done
 for path in bin migrations web/static build-manifest.json; do
  if [ -e "$APP/$path" ]; then
   install -d "$DOB_BACKUP/runtime/$(dirname "$path")"
   cp -a "$APP/$path" "$DOB_BACKUP/runtime/$path"
  fi
 done
 : > "$DOB_BACKUP/active"
 : > "$DOB_BACKUP/enabled"
 for unit in "${SERVICES[@]}"; do
  if systemctl is-active --quiet "$unit"; then echo "$unit" >> "$DOB_BACKUP/active"; fi
  if systemctl is-enabled --quiet "$unit"; then echo "$unit" >> "$DOB_BACKUP/enabled"; fi
 done
}
dob_restore_runtime_assets() {
 local path
 for path in "${DOB_UNIT_ASSETS[@]}"; do
  if [ -e "$DOB_BACKUP/units/$path" ]; then
   install -d "$DOB_UNIT_DIR/$(dirname "$path")" || return
   cp -a "$DOB_BACKUP/units/$path" "$DOB_UNIT_DIR/$path" || return
  else
   rm -f -- "$DOB_UNIT_DIR/$path" || return
  fi
 done
 for path in bin migrations web/static build-manifest.json; do
  rm -rf -- "$APP/$path" || return
  if [ -e "$DOB_BACKUP/runtime/$path" ]; then
   install -d "$APP/$(dirname "$path")" || return
   cp -a "$DOB_BACKUP/runtime/$path" "$APP/$path" || return
  fi
 done
}
# Disable newly enabled units while their files still exist. Otherwise systemd
# can leave dangling enable links after restoring an absent previous unit.
dob_restore_enablement_before_assets() {
 local unit load
 for unit in "${SERVICES[@]}"; do
  if ! grep -Fxq "$unit" "$DOB_BACKUP/enabled"; then
   load=$(systemctl show "$unit" -p LoadState --value) || return
   case "$load" in
    loaded)
     if ! systemctl disable "$unit"; then echo "Rollback disable failed: $unit" >&2; return 1; fi ;;
    not-found) ;;
    *) return 1 ;;
   esac
  fi
 done
}
dob_restore_runtime_services() {
 local unit
 while IFS= read -r unit; do
  if ! systemctl enable "$unit"; then echo "Rollback enable failed: $unit" >&2; return 1; fi
  if ! systemctl is-enabled --quiet "$unit"; then return 1; fi
 done < "$DOB_BACKUP/enabled"
 while IFS= read -r unit; do
  if ! systemctl start "$unit"; then echo "Rollback start failed: $unit" >&2; return 1; fi
  if ! systemctl is-active --quiet "$unit"; then return 1; fi
 done < "$DOB_BACKUP/active"
}
dob_rollback_runtime() {
 local code=$?
 trap - ERR
 set +e
 if ! dob_stop_runtime_readers; then
  echo "Rollback refused: cannot stop all runtime readers; evidence: $DOB_BACKUP" >&2
  exit "$code"
 fi
 # Never start checkpoint-unaware prior code over unresolved native work.
 if [ -f "$DOB_BACKUP/runtime/bin/digital-ocean-bot-worker" ]; then
  if ! (python3 - "$SRC/deploy/bootstrap.py" "$ETC" <<'GUARD'
import subprocess,runpy,sys,pathlib
# DOB_CHECKPOINT_ROLLBACK_GUARD
url=runpy.run_path(sys.argv[1])['read_environment'](pathlib.Path(sys.argv[2])/'env')['DATABASE_URL']
def q(sql):
 p=subprocess.run(['psql',url,'-XAt','-v','ON_ERROR_STOP=1','-c',sql],capture_output=True,text=True,timeout=10)
 if p.returncode:raise SystemExit('Cannot establish safe checkpoint rollback')
 return p.stdout.strip()
# All API/worker processes are stopped. Supported tuning CLI writers also
# acquire the shared host deploy lock, excluding starts during this guard.
if q("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='residential_performance_experiments' AND column_name='tuning')")=='t':
 if q("SELECT count(*) FROM residential_performance_experiments WHERE tuning->>'phase' IN('TESTING','PUBLISHING','RESTORING')")!='0':
  raise SystemExit('Unresolved residential tuning: overlay-unaware prior runtime remains stopped')
if q("SELECT to_regclass('residential_performance_panels') IS NULL")!='t':
 if q("SELECT count(*) FROM residential_performance_panels WHERE COALESCE(jsonb_array_length(config->'excluded_proxy_ids'),0)>0")!='0':
  raise SystemExit('Unrestored admission assignment: prior runtime remains stopped')
if q("SELECT to_regclass('worker_recovery_checkpoints') IS NULL")!='t':
 if q('SELECT count(*) FROM worker_recovery_checkpoints')!='0':raise SystemExit('Unresolved checkpoints: prior runtime remains stopped')
if q("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='deployment_step_attempts' AND column_name='result_snapshot')")=='t':
 if q("""SELECT count(*) FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id
 WHERE a.result_snapshot IS NOT NULL AND a.step=d.current_step
 AND a.generation=CASE WHEN a.step IN ('database','panel') THEN d.postinstall_generation ELSE 0 END
 AND d.state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','WAITING_INSTALLER')""")!='0':
  raise SystemExit('Unapplied workflow results: prior runtime remains stopped')
GUARD
  ); then
   echo "Rollback refused because checkpoint safety is unproven. Services remain stopped; new artifacts and backup retained: $DOB_BACKUP" >&2
   exit "$code"
  fi
 fi
 if ! dob_restore_enablement_before_assets; then
  echo "Rollback enablement restoration failed; managed services remain stopped. Evidence: $DOB_BACKUP" >&2
  exit "$code"
 fi
 if ! dob_restore_runtime_assets; then
  echo "Rollback restore failed; services remain stopped. Evidence: $DOB_BACKUP" >&2
  exit "$code"
 fi
 local unit
 if ! systemctl daemon-reload; then
  echo "Rollback daemon-reload failed; services remain stopped" >&2
  exit "$code"
 fi
 if ! dob_restore_runtime_services; then
  if dob_stop_runtime_readers; then
   echo "Rollback service restoration failed; managed services remain stopped. Evidence: $DOB_BACKUP" >&2
  else
   echo "Rollback service restoration failed and stop failed; inspect runtime immediately. Evidence: $DOB_BACKUP" >&2
  fi
  exit "$code"
 fi
 echo "Deployment failed; restored matching binaries, migrations, static, manifest and units. Evidence: $DOB_BACKUP" >&2
 exit "$code"
}
dob_verify_service_topology() {
 local role unit
 for unit in "${SERVICES[@]}"; do
  systemctl is-enabled --quiet "$unit"
  systemctl is-active --quiet "$unit"
 done
 for role in control panels; do
  unit=digital-ocean-bot-worker
  if [ "$role" = panels ]; then unit=digital-ocean-bot-worker-panels; fi
  systemctl is-active --quiet "$unit"
  test "$(systemctl show "$unit" -p Type --value)" = notify
  test "$(systemctl show "$unit" -p WatchdogUSec --value)" = 45s
  dob_verify_exact_exec "$unit" "$APP/bin/digital-ocean-bot-worker --role=$role"
 done
 systemctl is-active --quiet digital-ocean-bot-api
 dob_verify_exact_exec digital-ocean-bot-api "$APP/bin/digital-ocean-bot-api"
 python3 - <<'PY'
import json,time,urllib.request,subprocess,pathlib
# DOB_TOPOLOGY_RUNTIME_CONTRACT
pid=subprocess.check_output(['systemctl','show','digital-ocean-bot-api','-p','MainPID','--value'],text=True).strip()
mode=[x for x in pathlib.Path('/proc/'+pid+'/environ').read_bytes().split(b'\x00') if x.startswith(b'DOB_WORKER_MODE=')]
assert mode==[b'DOB_WORKER_MODE=split'],'API effective worker mode must be split'
end=time.monotonic()+60
last=None
while time.monotonic()<end:
 try:
  with urllib.request.urlopen('http://127.0.0.1:18080/readyz',timeout=3) as response:d=json.load(response)
  if len(d.get('checks',[]))==5 and all(x.get('ok') for x in d['checks']):break
 except Exception as exc:last=type(exc).__name__
 time.sleep(1)
else:raise SystemExit('split topology readiness did not converge: '+str(last))
PY
}
