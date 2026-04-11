#[unsafe(no_mangle)]
fn get_secret_number() -> u32 {
    1116
}

// Allocate a fixed size of memory and return the raw pointer
#[unsafe(no_mangle)]
fn allocate_memory(size: usize) -> *mut u8 {
    let vec = Vec::with_capacity(size);

    let (ptr, _len, _cap) = vec.into_raw_parts();

    let ptr = ptr as *mut u8;
    ptr
}

// Main filter function
#[unsafe(no_mangle)]
fn process_request(ptr: *const u8, len: usize) -> u8 {
    // Read slice
    let raw_data = unsafe {std::slice::from_raw_parts(ptr, len)};
    // Construct JSON
    let val: Result<serde_json::Value, _> = serde_json::from_slice(raw_data);
    // Check header
    match val {
        Ok(header) => {
            let is_suspicious = header.get("Block")
                .and_then(|values| values.get(0))
                .and_then(|first_val| first_val.as_str())
                .map(|s| s == "block")
                .unwrap_or(false);

            if is_suspicious { 1 } else { 0 }
        },
        Err(_) => {
            0
        }
    }
}

// Free memory
#[unsafe(no_mangle)]
fn free_memory(ptr : *mut u8, len: usize, cap: usize) {
    // The Vec goes out of scope and auto deallocated
    unsafe{ let _ = Vec::from_raw_parts(ptr, len, cap);}
}