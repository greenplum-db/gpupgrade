@test "we prefer xerrors.Errorf to errors.Wrap" {
  local directories=(hub/ agent/ cli/ utils/)

  for dir in "${directories[@]}"; do

    run git grep 'errors.Wrap' "$dir"

    if [ "$status" -eq 0 ]; then
      echo "found errors.Wrap usage in $dir: $output"
      exit 1
    fi

  done
}
