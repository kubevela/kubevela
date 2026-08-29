package example  
  
#Person: {  
    name!: string  
    age?:  int  
}  
  
person: #Person & {  
    name: "Alice"  
    age:  30  
}