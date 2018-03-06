until ./main; do
  echo "speedtestdog crashed with exit code $?.  Respawning in 5s.." >&2
  sleep 5
done
