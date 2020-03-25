mod client;

use client::GatewayClient;
use std::{collections::HashMap, env, error::Error, net::SocketAddr, sync::Arc, time::Duration};

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
    _last_stats: RwLock<Vec<client::Stat>>,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    let clusters = env::var("CLUSTERS")?;

    let server = Arc::new(Server {
        conns: Mutex::new(client::get_clients(clusters.parse()?).await?),
        listeners: Mutex::new(HashMap::new()),
        _last_stats: RwLock::new(Vec::new()),
    });

    {
        let srv = Arc::clone(&server);
        tokio::spawn(async move { srv.refresh_loop().await });
    }

    let mut listener = TcpListener::bind("0.0.0.0:80").await?;
    while let Ok((stream, addr)) = listener.accept().await {
        let srv = Arc::clone(&server);
        tokio::spawn(async move {
            if let Err(err) = srv.handle_connection(stream, addr).await {
                println!("failed to handle ws connection: {}", err)
            }
        });
    }

    Ok(())
}

impl Server {
    async fn handle_connection(
        &self,
        stream: TcpStream,
        addr: SocketAddr,
    ) -> Result<(), Box<dyn Error>> {
        println!("incoming connection from: {}", addr);
        let ws_stream = tokio_tungstenite::accept_async(stream).await?;
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
        Ok(())
    }

    async fn refresh_loop(&self) {
        let mut interval = tokio::time::interval(Duration::from_secs(10));

        loop {
            interval.tick().await;

            for conn in self.conns.lock().await.iter_mut() {
                let stat = conn.stats(client::EmptyRequest {}).await.map_or_else(
                    |err| {
                        println!("failed to check stats: {}", err);
                        client::StatsResponse { stats: Vec::new() }
                    },
                    |v| v.into_inner(),
                );

                dbg!(stat);
            }
        }
    }
}
