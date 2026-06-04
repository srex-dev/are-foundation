fn main() -> Result<(), Box<dyn std::error::Error>> {
    let protoc = protoc_bin_vendored::protoc_bin_path().map_err(|e| format!("protoc: {e}"))?;
    std::env::set_var("PROTOC", protoc);
    tonic_prost_build::compile_protos("proto/immutable_ledger.proto")?;
    Ok(())
}
