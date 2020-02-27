mod client;

use client::GatewayClient;

use futures_channel::mpsc::{unbounded, UnboundedSender};
use std::collections::HashMap;
use std::error::Error;
use std::net::SocketAddr;
use std::sync::Arc;
use std::sync::Mutex;
use tungstenite::protocol::Message;

type Tx = UnboundedSender<Message>;
type PeerMap = Arc<Mutex<HashMap<SocketAddr, Tx>>>;

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    let conns: Vec<GatewayClient> = Vec::new();
    let listeners = PeerMap::new(Mutex::new(HashMap::new()));

    println!("Hello, world!");
    Ok(())
}

fn refresh_loop() {}
