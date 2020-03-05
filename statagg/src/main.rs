mod client;

use client::GatewayClient;
use std::{collections::HashMap, error::Error, net::SocketAddr, sync::Arc, time::Duration};

use futures_channel::mpsc::{unbounded, UnboundedSender};
use futures_util::{future, pin_mut, stream::TryStreamExt, StreamExt};

use tokio::{
    net::{TcpListener, TcpStream},
    sync::{Mutex, RwLock},
};

use tungstenite::protocol::Message;

type Tx = UnboundedSender<Message>;

struct Server {
    conns: Mutex<Vec<GatewayClient>>,
    listeners: Mutex<HashMap<SocketAddr, Tx>>,
    _last_stat: RwLock<()>,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    let server = Arc::new(Server {
        conns: Mutex::new(Vec::new()),
        listeners: Mutex::new(HashMap::new()),
        _last_stat: RwLock::new(()),
    });

    {
        let srv = Arc::clone(&server);
        tokio::spawn(async move { srv.refresh_loop().await });
    }

    let mut listener = TcpListener::bind("127.0.0.1").await?;
    while let Ok((stream, addr)) = listener.accept().await {
        let srv = Arc::clone(&server);
        tokio::spawn(async move {
            srv.handle_connection(stream, addr).await;
        });
    }

    Ok(())
}

impl Server {
    async fn handle_connection(&self, stream: TcpStream, addr: SocketAddr) {
        println!("incoming connection from: {}", addr);
        let ws_stream = tokio_tungstenite::accept_async(stream)
            .await
            .expect("failed to accept websocket");
        println!("ws conn established with consumer: {}", addr);

        let (tx, rx) = unbounded();
        self.listeners.lock().await.insert(addr, tx);

        let (outgoing, incoming) = ws_stream.split();
        let ignore_incoming = incoming.try_for_each(|_| future::ok(()));
        let forward_outgoing = rx.map(Ok).forward(outgoing);

        pin_mut!(ignore_incoming, forward_outgoing);
        future::select(ignore_incoming, forward_outgoing).await;
        println!("user disconnected: {}", addr);
        self.listeners.lock().await.remove(&addr);
    }

    async fn refresh_loop(&self) {
        let mut interval = tokio::time::interval(Duration::from_secs(10));

        loop {
            interval.tick().await;

            for conn in self.conns.lock().await.iter_mut() {
                let stat = conn.stats(client::EmptyRequest {}).await.map_or_else(
                    |err| {
                        println!("failed to check stats: {}", err);
                        client::StatsResponse {}
                    },
                    |v| v.into_inner(),
                );

                dbg!(stat);
            }
        }
    }
}
