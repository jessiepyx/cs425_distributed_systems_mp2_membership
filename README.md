To run program:

    go run main.go

To clean directory:

    bash clean.sh

While program running...

- To print membership list:

      ls
    
- To join the group:

      join
    
- To leave the group:

      leave
    
- To display localhost id:

      id
    
- To display localhost ip:

      ip

To grep log (distributed query):

- On server side:

      go run server/server.go
    
- On client side:

      go run client/client.go <query pattern>

    Logs are in `logfile.log`. Grep Results are in `grep.out`.