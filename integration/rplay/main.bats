@test "rplay prints help" {
  run ./rplay
  [ "$status" -eq 0 ]
  [[ "$output" =~ "USAGE:" ]]
}

@test "rplay creates custom index path" {
  tdir=$(mktemp -d)
  export HOME="$tdir"
  run ./rplay search
  [ -d "$tdir/.local/share/rplay" ]
  [ "$status" -eq 0 ]
}


@test "rplay with custom index path" {
  tdir=$(mktemp -d)
  run ./rplay --index-path "$tdir"
  [ -d "$tdir" ]
  [ "$status" -eq 0 ]
}
